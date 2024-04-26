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

func NewInitializeConditions() action.Action[rhtasv1alpha1.Rekor] {
	return &initializeConditions{}
}

type initializeConditions struct {
	action.BaseAction
}

func (i initializeConditions) Name() string {
	return "pending"
}

func (i initializeConditions) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c == nil
}

func (i initializeConditions) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.Ready,
		Status: metav1.ConditionFalse,
		Reason: constants.Pending,
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   ServerCondition,
		Status: metav1.ConditionUnknown,
		Reason: constants.Pending,
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   RedisCondition,
		Status: metav1.ConditionUnknown,
		Reason: constants.Pending,
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   SignerCondition,
		Status: metav1.ConditionUnknown,
		Reason: constants.Pending,
	})
	if !utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   UICondition,
			Status: metav1.ConditionUnknown,
			Reason: constants.Pending,
		})
	}
	return i.StatusUpdate(ctx, instance)
}
