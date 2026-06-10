package tls

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	tlsutils "github.com/securesign/operator/internal/utils/tls"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewAction[T apis.ConditionsAwareObject](
	conditionType string,
	conditionResolvedStatus metav1.ConditionStatus,
	secretNameFormat string,
	component string,
	wrapper func(T) *wrapper[T],
) action.Action[T] {
	return &tlsAction[T]{
		conditionType:           conditionType,
		conditionResolvedStatus: conditionResolvedStatus,
		secretNameFormat:        secretNameFormat,
		component:               component,
		wrapper:                 wrapper,
	}
}

type tlsAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	conditionType           string
	conditionResolvedStatus metav1.ConditionStatus
	secretNameFormat        string
	component               string
	wrapper                 func(T) *wrapper[T]
}

func (i tlsAction[T]) Name() string {
	return fmt.Sprintf("resolve %s TLS", i.component)
}

func (i tlsAction[T]) CanHandle(_ context.Context, instance T) bool {
	w := i.wrapper(instance)
	c := meta.FindStatusCondition(instance.GetConditions(), i.conditionType)

	switch {
	case c == nil:
		return false
	case !w.IsEnabled():
		return false
	case c.Reason == state.Pending.String():
		return true
	case !equality.Semantic.DeepDerivative(w.SpecTLS(), w.StatusTLS()):
		return true
	default:
		return kubernetes.IsOpenShift() && w.StatusTLS().CertRef == nil
	}
}

func (i tlsAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	w := i.wrapper(instance)

	switch {
	case w.SpecTLS().CertRef != nil:
		w.SetStatusTLS(w.SpecTLS())
	case kubernetes.IsOpenShift():
		w.SetStatusTLS(rhtasv1.TLS{
			CertRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: fmt.Sprintf(i.secretNameFormat, instance.GetName())},
				Key:                  tlsutils.KeyCert,
			},
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: fmt.Sprintf(i.secretNameFormat, instance.GetName())},
				Key:                  tlsutils.KeyPrivate,
			},
		})
	default:
		i.Logger.V(1).Info(fmt.Sprintf("Communication to %s is insecure", i.component))
	}

	instance.SetCondition(metav1.Condition{
		Type:    i.conditionType,
		Status:  i.conditionResolvedStatus,
		Reason:  ReasonResolved,
		Message: "TLS configuration resolved",
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
