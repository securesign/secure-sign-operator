package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &tlsAction{}
}

type tlsAction struct {
	action.BaseAction
}

func (i tlsAction) Name() string {
	return "resolve server TLS"
}

func (i tlsAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	return !meta.IsStatusConditionTrue(instance.Status.Conditions, TLSCondition) || !equality.Semantic.DeepDerivative(instance.Spec.TLS, instance.Status.TLS)
}

func (i tlsAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	// TLS
	switch {
	case instance.Spec.TLS.CertRef != nil:
		if cryptoutil.FIPSEnabled {
			if err := cryptoutil.ValidateTLS(i.Client, instance.Namespace, instance.Spec.TLS); err != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    TLSCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: fmt.Sprintf("TLS material is not FIPS-compliant: %v", err),
				})
				i.StatusUpdate(ctx, instance)
				return i.Requeue()
			}
		}
		instance.Status.TLS = instance.Spec.TLS
	case kubernetes.IsOpenShift():
		instance.Status.TLS = rhtasv1alpha1.TLS{
			CertRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(TLSSecret, instance.Name)},
				Key:                  "tls.crt",
			},
			PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(TLSSecret, instance.Name)},
				Key:                  "tls.key",
			},
		}
	default:
		i.Logger.V(1).Info("Communication to CTLog is insecure")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    TLSCondition,
			Status:  metav1.ConditionTrue,
			Reason:  "NotProvided",
			Message: "Communication to CTLog is insecure",
		})
		return i.StatusUpdate(ctx, instance)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    TLSCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "TLSResolved",
		Message: "TLS configuration resolved",
	})
	return i.StatusUpdate(ctx, instance)
}
