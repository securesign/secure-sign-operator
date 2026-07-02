package transitions

import (
	"context"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewEnsureConditionsAction[T apis.ConditionsAwareObject](componentSupplier ComponentSupplier[T]) action.Action[T] {
	return &ensureConditions[T]{componentSupplier: componentSupplier}
}

type ensureConditions[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	componentSupplier func(T) []string
}

func (i ensureConditions[T]) Name() string {
	return "ensure conditions"
}

func (i ensureConditions[T]) CanHandle(_ context.Context, instance T) bool {
	for _, c := range i.componentSupplier(instance) {
		if meta.FindStatusCondition(instance.GetConditions(), c) == nil {
			return true
		}
	}
	return false
}

func (i ensureConditions[T]) Handle(ctx context.Context, instance T) *action.Result {
	for _, c := range i.componentSupplier(instance) {
		if meta.FindStatusCondition(instance.GetConditions(), c) == nil {
			instance.SetCondition(metav1.Condition{
				Type:   c,
				Status: metav1.ConditionUnknown,
				Reason: state.Pending.String(),
			})
		}
	}
	return i.StatusUpdate(ctx, instance)
}
