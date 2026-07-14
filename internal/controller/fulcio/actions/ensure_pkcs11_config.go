package actions

import (
	"context"
	"fmt"
	"reflect"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
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
	if instance.Spec.Certificate.CAType != rhtasv1.CATypePKCS11 {
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
	// Fire if drift detected between spec and status
	return i.driftDetected(instance)
}

func (i ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	pkcs11Config := instance.Spec.Certificate.PKCS11
	if pkcs11Config == nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("pkcs11 config is required when caType is pkcs11")), instance)
	}

	// Validate credentialsRef Secret exists
	exists, err := kubernetes.ExistsSecret(ctx, i.Client, instance.Namespace, pkcs11Config.CredentialsRef.Name)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not check credentialsRef Secret: %w", err), instance)
	}
	if !exists {
		return i.Error(ctx, fmt.Errorf("credentialsRef Secret %q not found", pkcs11Config.CredentialsRef.Name), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             "SecretNotFound",
				Message:            fmt.Sprintf("credentialsRef Secret %q not found", pkcs11Config.CredentialsRef.Name),
				ObservedGeneration: instance.Generation,
			},
		)
	}

	// Validate pkcs11ConfigRef Secret exists
	exists, err = kubernetes.ExistsSecret(ctx, i.Client, instance.Namespace, pkcs11Config.PKCS11ConfigRef.Name)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not check pkcs11ConfigRef Secret: %w", err), instance)
	}
	if !exists {
		return i.Error(ctx, fmt.Errorf("pkcs11ConfigRef Secret %q not found", pkcs11Config.PKCS11ConfigRef.Name), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             "SecretNotFound",
				Message:            fmt.Sprintf("pkcs11ConfigRef Secret %q not found", pkcs11Config.PKCS11ConfigRef.Name),
				ObservedGeneration: instance.Generation,
			},
		)
	}

	// Copy spec refs to status
	if instance.Status.PKCS11 == nil {
		instance.Status.PKCS11 = &rhtasv1.FulcioPKCS11Status{}
	}
	instance.Status.PKCS11.CredentialsRef = pkcs11Config.CredentialsRef.DeepCopy()
	instance.Status.PKCS11.PKCS11ConfigRef = pkcs11Config.PKCS11ConfigRef.DeepCopy()

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            "PKCS#11 configuration secrets validated",
		ObservedGeneration: instance.Generation,
	})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

// driftDetected compares spec PKCS#11 config references with status to detect changes.
func (i ensurePKCS11Config) driftDetected(instance *rhtasv1.Fulcio) bool {
	if instance.Spec.Certificate.PKCS11 == nil || instance.Status.PKCS11 == nil {
		return true
	}
	specPKCS11 := instance.Spec.Certificate.PKCS11
	statusPKCS11 := instance.Status.PKCS11

	if statusPKCS11.CredentialsRef == nil || !reflect.DeepEqual(specPKCS11.CredentialsRef, *statusPKCS11.CredentialsRef) {
		return true
	}
	if statusPKCS11.PKCS11ConfigRef == nil || !reflect.DeepEqual(specPKCS11.PKCS11ConfigRef, *statusPKCS11.PKCS11ConfigRef) {
		return true
	}
	return false
}
