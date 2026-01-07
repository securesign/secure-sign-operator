package transitions

import (
	"context"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToCreatePhaseAction[T apis.ConditionsAwareObject]() action.Action[T] {
	return &toCreate[T]{}
}

type toCreate[T apis.ConditionsAwareObject] struct {
	action.BaseAction
}

func (i toCreate[T]) Name() string {
	return "move to create phase"
}

func (i toCreate[T]) CanHandle(_ context.Context, instance T) bool {
	return state.FromInstance(instance, constants.ReadyCondition) == state.Pending
}

func (i toCreate[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{Type: constants.ReadyCondition,
		Status: metav1.ConditionFalse, Reason: state.Creating.String(),
		ObservedGeneration: instance.GetGeneration()})
	return i.StatusUpdate(ctx, instance)
}
