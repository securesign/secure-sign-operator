package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewEnsurePKCS11ConfigAction() action.Action[*rhtasv1.CTlog] {
	return &ensurePKCS11Config{}
}

type ensurePKCS11Config struct {
	action.BaseAction
}

func (a ensurePKCS11Config) Name() string {
	return "ensure PKCS#11 config"
}

func (a ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}

	if !hasPKCS11Log(instance) {
		return false
	}

	c := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if c == nil || c.Status != metav1.ConditionTrue {
		return true
	}

	currentHash := allPKCS11SpecHash(instance)
	storedHash := instance.GetAnnotations()[annotations.PKCS11SpecHash]
	return currentHash != storedHash
}

func (a ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	for logIdx, log := range instance.Spec.Logs {
		if log.Signer == nil || log.Signer.Type != rhtasv1.SignerTypePKCS11 || log.Signer.PKCS11 == nil {
			continue
		}
		p := log.Signer.PKCS11

		if err := a.validateSecretRef(ctx, instance, p.PinSecretRef,
			fmt.Sprintf("spec.logs[%d].signer.pkcs11.pinSecretRef", logIdx)); err != nil {
			if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
				return a.Error(ctx, persistErr, instance)
			}
			return a.RequeueAfter(5 * time.Second)
		}

		if err := a.validateSecretRef(ctx, instance, p.PublicKeyRef,
			fmt.Sprintf("spec.logs[%d].signer.pkcs11.publicKeyRef", logIdx)); err != nil {
			if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
				return a.Error(ctx, persistErr, instance)
			}
			return a.RequeueAfter(5 * time.Second)
		}

		if log.Active != nil && *log.Active {
			instance.Status.PublicKeyRef = p.PublicKeyRef.DeepCopy()
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               ConfigCondition,
		Status:             metav1.ConditionFalse,
		Reason:             "PKCS11Config",
		Message:            "PKCS#11 configuration changed",
		ObservedGeneration: instance.Generation,
	})

	contentHash := allPKCS11SpecHash(instance)
	ann := instance.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[annotations.PKCS11SpecHash] = contentHash
	instance.SetAnnotations(ann)

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            PKCS11MessageResolved,
		ObservedGeneration: instance.Generation,
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

func (a ensurePKCS11Config) validateSecretRef(
	ctx context.Context,
	instance *rhtasv1.CTlog,
	ref *rhtasv1.SecretKeySelector,
	fieldName string,
) error {
	if ref == nil {
		err := fmt.Errorf("%s is nil", fieldName)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               PKCS11Condition,
			Status:             metav1.ConditionFalse,
			Reason:             "Missing",
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return err
	}

	data, err := kubernetes.GetSecretData(ctx, a.Client, instance.Namespace, ref)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               PKCS11Condition,
			Status:             metav1.ConditionFalse,
			Reason:             "SecretNotFound",
			Message:            fmt.Sprintf("failed to read %s secret %s/%s: %v", fieldName, ref.Name, ref.Key, err),
			ObservedGeneration: instance.Generation,
		})
		return err
	}

	if len(data) == 0 {
		err := fmt.Errorf("%s secret %s key %s is empty", fieldName, ref.Name, ref.Key)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               PKCS11Condition,
			Status:             metav1.ConditionFalse,
			Reason:             "EmptySecret",
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return err
	}

	return nil
}

func hasPKCS11Log(instance *rhtasv1.CTlog) bool {
	for _, log := range instance.Spec.Logs {
		if log.Signer != nil && log.Signer.Type == rhtasv1.SignerTypePKCS11 {
			return true
		}
	}
	return false
}

func allPKCS11SpecHash(instance *rhtasv1.CTlog) string {
	h := sha256.New()
	for _, log := range instance.Spec.Logs {
		if log.Signer == nil || log.Signer.PKCS11 == nil {
			continue
		}
		fmt.Fprintf(h, "prefix:%s\n", log.Prefix)
		fmt.Fprintf(h, "modulePath:%s\n", log.Signer.PKCS11.ModulePath)
		fmt.Fprintf(h, "tokenLabel:%s\n", log.Signer.PKCS11.TokenLabel)
		if log.Signer.PKCS11.PinSecretRef != nil {
			fmt.Fprintf(h, "pinSecretRef:%s/%s\n", log.Signer.PKCS11.PinSecretRef.Name, log.Signer.PKCS11.PinSecretRef.Key)
		}
		if log.Signer.PKCS11.PublicKeyRef != nil {
			fmt.Fprintf(h, "publicKeyRef:%s/%s\n", log.Signer.PKCS11.PublicKeyRef.Name, log.Signer.PKCS11.PublicKeyRef.Key)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func pkcs11SpecHash(p *rhtasv1.CTlogPKCS11Config) string {
	if p == nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "modulePath:%s\n", p.ModulePath)
	fmt.Fprintf(h, "tokenLabel:%s\n", p.TokenLabel)
	if p.PinSecretRef != nil {
		fmt.Fprintf(h, "pinSecretRef:%s/%s\n", p.PinSecretRef.Name, p.PinSecretRef.Key)
	}
	if p.PublicKeyRef != nil {
		fmt.Fprintf(h, "publicKeyRef:%s/%s\n", p.PublicKeyRef.Name, p.PublicKeyRef.Key)
	}
	return hex.EncodeToString(h.Sum(nil))
}
