package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Rekor] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	if (instance.Spec.RekorSearchUI.Enabled == nil || !*instance.Spec.RekorSearchUI.Enabled || meta.IsStatusConditionTrue(instance.Status.Conditions, UICondition)) &&
		meta.IsStatusConditionTrue(instance.Status.Conditions, RedisCondition) &&
		meta.IsStatusConditionTrue(instance.Status.Conditions, ServerCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionTrue, Reason: constants.Ready})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Requeue()
}
