package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	utils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPendingAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &pendingAction{}
}

type pendingAction struct {
	action.BaseAction
}

func (i pendingAction) Name() string {
	return "pending"
}

func (i pendingAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason == constants.Pending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var err error
	_, err = utils.GetInternalUrl(ctx, i.Client, instance.Namespace, trillian.LogserverDeploymentName)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "Waiting for Trillian Logserver service",
		})
		// update will throw requeue only with first update
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}
	return i.Continue()
}

func (i pendingAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.CTlog) bool {
	return false
}

func (i pendingAction) HandleError(_ context.Context, _ *rhtasv1alpha1.CTlog) *action.Result {
	return i.Continue()
}
