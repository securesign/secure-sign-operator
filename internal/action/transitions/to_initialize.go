package transitions

import (
	"context"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToInitializePhaseAction[T apis.ConditionsAwareObject]() action.Action[T] {
	return &toInitializeAction[T]{}
}

type toInitializeAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
}

func (i toInitializeAction[T]) Name() string {
	return "move to initialization phase"
}

func (i toInitializeAction[T]) CanHandle(_ context.Context, instance T) bool {
	return state.FromInstance(instance, constants.ReadyCondition) == state.Creating
}

func (i toInitializeAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{Type: constants.ReadyCondition,
		Status: metav1.ConditionFalse, Reason: state.Initialize.String(),
		ObservedGeneration: instance.GetGeneration()})
	return i.StatusUpdate(ctx, instance)
}
