package logsigner

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &tlsAction{}
}

type tlsAction struct {
	action.BaseAction
}

func (i tlsAction) Name() string {
	return "resolve TLS"
}

func (i tlsAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, actions.SignerCondition)

	switch {
	case c == nil:
		return false
	case c.Reason == constants.Pending:
		return true
	case !equality.Semantic.DeepDerivative(specTLS(instance), statusTLS(instance)):
		return true
	default:
		// enable TLS on OCP by default
		return c.Reason == constants.Ready && kubernetes.IsOpenShift() && statusTLS(instance).CertRef == nil
	}
}

func (i tlsAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	switch {
	case specTLS(instance).CertRef != nil:
		if cryptoutil.FIPSEnabled {
			if err := cryptoutil.ValidateTLS(i.Client, instance.Namespace, specTLS(instance)); err != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    actions.SignerCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: fmt.Sprintf("TLS material is not FIPS-compliant: %v", err),
				})
				i.StatusUpdate(ctx, instance)
				return i.Requeue()
			}
		}
		setStatusTLS(instance, specTLS(instance))
	case kubernetes.IsOpenShift():
		setStatusTLS(instance, rhtasv1alpha1.TLS{
			CertRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(actions.LogSignerTLSSecret, instance.Name)},
				Key:                  "tls.crt",
			},
			PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(actions.LogSignerTLSSecret, instance.Name)},
				Key:                  "tls.key",
			},
		})
	default:
		i.Logger.V(1).Info("Communication to trillian log signer is insecure")
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.SignerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  "TLSResolved",
		Message: "TLS configuration resolved",
	})
	return i.StatusUpdate(ctx, instance)
}
