package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewEnsurePKCS11ConfigAction() action.Action[*rhtasv1.Fulcio] {
	return &ensurePKCS11Config{}
}

type ensurePKCS11Config struct {
	action.BaseAction
}

func (i ensurePKCS11Config) Name() string {
	return "ensure pkcs11 config"
}

func (i ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	if instance.Spec.Signer.Type != rhtasv1.FulcioSignerTypePKCS11 {
		return false
	}
	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if cond == nil {
		return true
	}
	if cond.ObservedGeneration != instance.GetGeneration() {
		return true
	}
	return false
}

func (i ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	// Handle rotation: if condition was already True and generation changed,
	// invalidate dependent conditions to trigger server config regeneration
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if cond != nil && cond.Status == metav1.ConditionTrue && cond.ObservedGeneration != instance.GetGeneration() {
		return i.handleRotation(ctx, instance)
	}

	pkcs11Config := instance.Spec.Signer.PKCS11
	if pkcs11Config == nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("pkcs11 config is required when type is pkcs11")), instance)
	}

	// Validate configRef Secret exists and contains the expected key
	_, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, &pkcs11Config.ConfigRef)
	if err != nil {
		msg := fmt.Sprintf("configRef Secret %q key %q not accessible: %v", pkcs11Config.ConfigRef.Name, pkcs11Config.ConfigRef.Key, err)
		return i.Error(ctx, fmt.Errorf("%s", msg), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             "SecretNotFound",
				Message:            msg,
				ObservedGeneration: instance.Generation,
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            msg,
				ObservedGeneration: instance.Generation,
			},
		)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            "PKCS#11 configuration secrets validated",
		ObservedGeneration: instance.Generation,
	})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func (i ensurePKCS11Config) handleRotation(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: PKCS11Condition, Status: metav1.ConditionFalse,
		Reason: "Rotation", Message: "PKCS#11 configuration changed",
		ObservedGeneration: instance.Generation,
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: constants.ReadyCondition, Status: metav1.ConditionFalse,
		Reason: state.Pending.String(), ObservedGeneration: instance.Generation,
	})

	i.Recorder.Eventf(instance, nil, core.EventTypeNormal,
		"PKCS11RotationStarted", "Rotation",
		"PKCS#11 configuration changed, re-deploying Fulcio")

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
