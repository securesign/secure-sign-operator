package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	actions2 "github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.Rekor] {
	return &tlsAction{}
}

type tlsAction struct {
	action.BaseAction
}

func (i tlsAction) Name() string {
	return "resolve TLS"
}

func (i tlsAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, actions2.RedisCondition)

	switch {
	case c == nil:
		return false
	case !enabled(instance):
		return false
	case c.Reason == state.Pending.String():
		return true
	case !equality.Semantic.DeepDerivative(specTLS(instance), statusTLS(instance)):
		return true
	default:
		// enable TLS on OCP by default
		return c.Reason == state.Ready.String() && kubernetes.IsOpenShift() && statusTLS(instance).CertRef == nil
	}
}

func (i tlsAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	switch {
	case specTLS(instance).CertRef != nil:
		setStatusTLS(instance, specTLS(instance))
	case kubernetes.IsOpenShift():
		setStatusTLS(instance, common.TLS{
			CertRef: &common.SecretKeySelector{
				LocalObjectReference: common.LocalObjectReference{Name: fmt.Sprintf(actions2.RedisTlsSecret, instance.Name)},
				Key:                  "tls.crt",
			},
			PrivateKeyRef: &common.SecretKeySelector{
				LocalObjectReference: common.LocalObjectReference{Name: fmt.Sprintf(actions2.RedisTlsSecret, instance.Name)},
				Key:                  "tls.key",
			},
		})
	default:
		i.Logger.V(1).Info("Communication to redis server is insecure")
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions2.RedisCondition,
		Status:  metav1.ConditionFalse,
		Reason:  "TLSResolved",
		Message: "TLS configuration resolved",
	})
	return i.StatusUpdate(ctx, instance)
}
