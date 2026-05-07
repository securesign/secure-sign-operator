package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.CTlog] {
	return &tlsAction{}
}

type tlsAction struct {
	action.BaseAction
}

func (i tlsAction) Name() string {
	return "resolve server TLS"
}

func (i tlsAction) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return !meta.IsStatusConditionTrue(instance.Status.Conditions, TLSCondition) || !equality.Semantic.DeepDerivative(instance.Spec.TLS, instance.Status.TLS)
}

func (i tlsAction) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	// TLS
	switch {
	case instance.Spec.TLS.CertRef != nil:
		instance.Status.TLS = instance.Spec.TLS
	case kubernetes.IsOpenShift():
		instance.Status.TLS = common.TLS{
			CertRef: &common.SecretKeySelector{
				LocalObjectReference: common.LocalObjectReference{Name: fmt.Sprintf(TLSSecret, instance.Name)},
				Key:                  "tls.crt",
			},
			PrivateKeyRef: &common.SecretKeySelector{
				LocalObjectReference: common.LocalObjectReference{Name: fmt.Sprintf(TLSSecret, instance.Name)},
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
