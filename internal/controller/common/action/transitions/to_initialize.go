package transitions

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
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
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Creating
}

func (i toInitializeAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Initialize})

	return i.StatusUpdate(ctx, instance)
}
