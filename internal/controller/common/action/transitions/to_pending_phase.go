package transitions

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ComponentSupplier[T apis.ConditionsAwareObject] func(T) []string

func NewToPendingPhaseAction[T apis.ConditionsAwareObject](componentSupplier ComponentSupplier[T]) action.Action[T] {
	return &toPending[T]{componentSupplier: componentSupplier}
}

type toPending[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	componentSupplier func(T) []string
}

func (i toPending[T]) Name() string {
	return "move to pending phase"
}

func (i toPending[T]) CanHandle(_ context.Context, instance T) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return c == nil || c.Status == metav1.ConditionUnknown
}

func (i toPending[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Pending})

	for _, c := range i.componentSupplier(instance) {
		instance.SetCondition(metav1.Condition{Type: c,
			Status: metav1.ConditionUnknown, Reason: constants.Pending})
	}
	return i.StatusUpdate(ctx, instance)
}

func (i toPending[T]) CanHandleError(_ context.Context, _ T) bool {
	return false
}

func (i toPending[T]) HandleError(_ context.Context, _ T) *action.Result {
	// NO-OP
	return i.Continue()
}
