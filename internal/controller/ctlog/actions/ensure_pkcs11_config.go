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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

// CanHandle fires only when Type==pkcs11 AND either:
//   - The component is in Creating (or later) state, AND
//   - PKCS11Condition is not yet set, OR
//   - The content hash of spec.logs[0].signer.pkcs11 fields differs from the hash
//     stored in the PKCS11Condition message (drift detection).
func (a ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	if len(instance.Spec.Logs) == 0 || instance.Spec.Logs[0].Signer == nil {
		return false
	}
	if instance.Spec.Logs[0].Signer.Type != rhtasv1.SignerTypePKCS11 {
		return false
	}

	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}

	c := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if c == nil || c.Status != metav1.ConditionTrue {
		return true
	}

	// Drift detection: compare current spec hash against stored annotation.
	currentHash := pkcs11SpecHash(instance.Spec.Logs[0].Signer.PKCS11)
	storedHash := instance.GetAnnotations()[annotations.PKCS11SpecHash]
	return currentHash != storedHash
}

// Handle validates that the PKCS#11 secret references exist and are readable,
// sets Status.PublicKeyRef for trust material resolution, invalidates
// ConfigCondition to trigger server config regeneration, and sets
// PKCS11Condition to True.
func (a ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	p := instance.Spec.Logs[0].Signer.PKCS11
	if p == nil {
		return a.Error(ctx,
			reconcile.TerminalError(fmt.Errorf("spec.logs[0].signer.pkcs11 is nil but signer type is pkcs11")),
			instance,
		)
	}

	// Validate PinSecretRef
	if err := a.validateSecretRef(ctx, instance, p.PinSecretRef, "spec.signer.pkcs11.pinSecretRef"); err != nil {
		if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
			return a.Error(ctx, persistErr, instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	// Validate PublicKeyRef
	if err := a.validateSecretRef(ctx, instance, p.PublicKeyRef, "spec.signer.pkcs11.publicKeyRef"); err != nil {
		if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
			return a.Error(ctx, persistErr, instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	// Set Status.PublicKeyRef for trust material resolution (resolve_pub_key action).
	instance.Status.PublicKeyRef = p.PublicKeyRef.DeepCopy()

	// Invalidate ConfigCondition to trigger server config regeneration.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               ConfigCondition,
		Status:             metav1.ConditionFalse,
		Reason:             "PKCS11Config",
		Message:            "PKCS#11 configuration changed",
		ObservedGeneration: instance.Generation,
	})

	// Store spec hash as annotation for drift detection.
	contentHash := pkcs11SpecHash(p)
	ann := instance.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[annotations.PKCS11SpecHash] = contentHash
	instance.SetAnnotations(ann)

	// Set PKCS11Condition to True.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            PKCS11MessageResolved,
		ObservedGeneration: instance.Generation,
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

// validateSecretRef validates a single SecretKeySelector by fetching the secret data.
// Returns the secret data on success, or sets PKCS11Condition to False and returns an error.
// This helper collapses the duplicated validation blocks for PinSecretRef and PublicKeyRef
// (addresses osmman review comment #6).
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

// pkcs11SpecHash computes a deterministic SHA-256 hash of the PKCS#11 spec fields
// used for drift detection. This avoids the bug where any ObservedGeneration change
// (e.g. monitoring toggle) would re-fire PKCS#11 validation (osmman issue #1).
func pkcs11SpecHash(p *rhtasv1.CTlogPKCS11Config) string {
	if p == nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "modulePath:%s\n", p.ModulePath) //nolint:errcheck // hash.Hash.Write never returns an error
	fmt.Fprintf(h, "tokenLabel:%s\n", p.TokenLabel) //nolint:errcheck
	if p.PinSecretRef != nil {
		fmt.Fprintf(h, "pinSecretRef:%s/%s\n", p.PinSecretRef.Name, p.PinSecretRef.Key) //nolint:errcheck
	}
	if p.PublicKeyRef != nil {
		fmt.Fprintf(h, "publicKeyRef:%s/%s\n", p.PublicKeyRef.Name, p.PublicKeyRef.Key) //nolint:errcheck
	}
	return hex.EncodeToString(h.Sum(nil))
}
