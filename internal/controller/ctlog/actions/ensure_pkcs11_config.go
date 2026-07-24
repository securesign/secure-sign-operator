package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func NewEnsurePKCS11ConfigAction() action.Action[*rhtasv1.CTlog] {
	return &ensurePKCS11Config{}
}

type ensurePKCS11Config struct {
	action.BaseAction
}

func (e ensurePKCS11Config) Name() string {
	return "ensure-pkcs11-config"
}

func (e ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	if instance.Spec.SignerType != rhtasv1.CTlogSignerTypePKCS11 {
		return false
	}
	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}
	// Fire if PKCS11 condition is not yet set
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if cond == nil {
		return true
	}
	// Fire if CR generation changed (e.g., tokenLabel updated)
	if cond.ObservedGeneration != instance.GetGeneration() {
		return true
	}
	// Fire if drift detected between spec and status
	return e.hasPKCS11ConfigDrift(instance)
}

func (e ensurePKCS11Config) hasPKCS11ConfigDrift(instance *rhtasv1.CTlog) bool {
	if instance.Status.PKCS11 == nil {
		return true
	}
	spec := instance.Spec.PKCS11
	status := instance.Status.PKCS11

	if !equality.Semantic.DeepDerivative(spec.PinSecretRef, status.PinSecretRef) {
		return true
	}
	if !equality.Semantic.DeepDerivative(spec.PublicKeyRef, status.PublicKeyRef) {
		return true
	}
	if spec.TokenLabel != status.TokenLabel {
		return true
	}
	if spec.PKCS11ModulePath != status.PKCS11ModulePath {
		return true
	}
	return false
}

func (e ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	if meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11Condition) {
		return e.handleRotation(ctx, instance)
	}

	p := instance.Spec.PKCS11
	if p == nil {
		return e.Error(ctx, fmt.Errorf("pkcs11 config is nil"), instance)
	}

	// Validate required refs
	if p.PinSecretRef == nil {
		return e.Error(ctx, fmt.Errorf("pinSecretRef must be specified for PKCS#11 mode"), instance,
			metav1.Condition{
				Type:    PKCS11Condition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: "pinSecretRef must be specified",
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            "PKCS#11 configuration incomplete",
				ObservedGeneration: instance.Generation,
			})
	}

	if p.PublicKeyRef == nil {
		return e.Error(ctx, fmt.Errorf("publicKeyRef must be specified for PKCS#11 mode"), instance,
			metav1.Condition{
				Type:    PKCS11Condition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: "publicKeyRef must be specified",
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            "PKCS#11 configuration incomplete",
				ObservedGeneration: instance.Generation,
			})
	}

	// Validate that referenced secrets exist
	if _, err := kubernetes.GetSecretData(ctx, e.Client, instance.Namespace, p.PinSecretRef); err != nil {
		msg := fmt.Sprintf("PinSecretRef not accessible: %v", err)
		return e.Error(ctx, fmt.Errorf("cannot read PinSecretRef: %w", err), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            msg,
				ObservedGeneration: instance.Generation,
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            msg,
				ObservedGeneration: instance.Generation,
			})
	}

	if _, err := kubernetes.GetSecretData(ctx, e.Client, instance.Namespace, p.PublicKeyRef); err != nil {
		msg := fmt.Sprintf("PublicKeyRef not accessible: %v", err)
		return e.Error(ctx, fmt.Errorf("cannot read PublicKeyRef: %w", err), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            msg,
				ObservedGeneration: instance.Generation,
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            msg,
				ObservedGeneration: instance.Generation,
			})
	}

	// Populate status
	instance.Status.PKCS11 = &rhtasv1.CTlogPKCS11Status{
		PinSecretRef:     p.PinSecretRef,
		PublicKeyRef:     p.PublicKeyRef,
		TokenLabel:       p.TokenLabel,
		PKCS11ModulePath: p.PKCS11ModulePath,
	}
	// Also set Status.PublicKeyRef so trust material resolution can find it
	// (HandleKeys is skipped in PKCS#11 mode, which normally sets this)
	instance.Status.PublicKeyRef = p.PublicKeyRef

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: instance.Generation,
	})

	// Invalidate server config so it gets regenerated with PKCS#11 settings
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               ConfigCondition,
		Status:             metav1.ConditionFalse,
		Reason:             "PKCS11ConfigChanged",
		Message:            "PKCS#11 config resolved, server config needs regeneration",
		ObservedGeneration: instance.Generation,
	})

	return e.ReturnOnChange(e.PersistStatus)(ctx, instance)
}

func (e ensurePKCS11Config) handleRotation(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: PKCS11Condition, Status: metav1.ConditionFalse,
		Reason: "Rotation", Message: "PKCS#11 configuration drift detected",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: ConfigCondition, Status: metav1.ConditionFalse,
		Reason: "Rotation", Message: "Server config needs regeneration",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: constants.ReadyCondition, Status: metav1.ConditionFalse,
		Reason: state.Pending.String(), ObservedGeneration: instance.Generation,
	})

	e.Recorder.Eventf(instance, nil, corev1.EventTypeNormal,
		"PKCS11RotationStarted", "Rotation",
		"Key rotation initiated, re-deploying CTlog")

	return e.ReturnOnChange(e.PersistStatus)(ctx, instance)
}
