package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewUpdateStatusAction() action.Action[rhtasv1alpha1.Securesign] {
	return &updateStatusAction{}
}

type updateStatusAction struct {
	action.BaseAction
}

func (i updateStatusAction) Name() string {
	return "update status"
}

func (i updateStatusAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) != nil
}

func (i updateStatusAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	for _, conditionType := range []string{TrillianCondition, FulcioCondition, RekorCondition, CTlogCondition, TufCondition} {
		c := meta.FindStatusCondition(instance.Status.Conditions, conditionType)
		if c.Status != v1.ConditionTrue {
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:    constants.Ready,
				Status:  c.Status,
				Reason:  c.Reason,
				Message: c.Message,
			})
			return i.StatusUpdate(ctx, instance)
		}
	}

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, constants.Ready) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   constants.Ready,
			Status: v1.ConditionTrue,
			Reason: constants.Ready,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
