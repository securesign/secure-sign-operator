package transitions

import (
	"context"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToReadyPhaseAction[T apis.ConditionsAwareObject]() action.Action[T] {
	return &toReady[T]{}
}

type toReady[T apis.ConditionsAwareObject] struct {
	action.BaseAction
}

func (i toReady[T]) Name() string {
	return "move to ready phase"
}

func (i toReady[T]) CanHandle(_ context.Context, instance T) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	if c == nil {
		return false
	}
	return c.Reason != state.Ready.String() ||
		c.Status != metav1.ConditionTrue ||
		c.ObservedGeneration != instance.GetGeneration()
}

func (i toReady[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{
		Type:               constants.ReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		ObservedGeneration: instance.GetGeneration(),
	})

	return i.StatusUpdate(ctx, instance)
}
