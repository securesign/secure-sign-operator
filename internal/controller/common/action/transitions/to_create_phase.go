package transitions

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
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
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Pending
}

func (i toCreate[T]) Handle(ctx context.Context, instance T) *action.Result {
	instance.SetCondition(metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating})
	return i.StatusUpdate(ctx, instance)
}
