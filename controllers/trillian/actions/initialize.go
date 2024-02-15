package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Trillian] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Initialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	if meta.IsStatusConditionTrue(instance.Status.Conditions, DbCondition) &&
		meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition) &&
		meta.IsStatusConditionTrue(instance.Status.Conditions, ServerCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionTrue, Reason: constants.Ready})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Requeue()

}
