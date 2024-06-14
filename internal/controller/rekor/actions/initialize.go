package actions

import (
	"context"

	"github.com/securesign/operator/internal/controller/common/utils"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return meta.IsStatusConditionFalse(instance.Status.Conditions, constants.Ready)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, ServerCondition) {
		return i.Requeue()
	}

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, RedisCondition) {
		return i.Requeue()
	}

	if utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled) {
		if !meta.IsStatusConditionTrue(instance.Status.Conditions, UICondition) {
			return i.Requeue()
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionTrue, Reason: constants.Ready})
	return i.StatusUpdate(ctx, instance)
}

func (i initializeAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.Rekor) bool {
	return false
}

func (i initializeAction) HandleError(_ context.Context, _ *rhtasv1alpha1.Rekor) *action.Result {
	return i.Continue()
}
