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

func NewPendingAction() action.Action[rhtasv1alpha1.CTlog] {
	return &pendingAction{}
}

type pendingAction struct {
	action.BaseAction
}

func (i pendingAction) Name() string {
	return "pending"
}

func (i pendingAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c == nil || c.Reason == constants.Pending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) == nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   CertCondition,
			Status: metav1.ConditionUnknown,
			Reason: constants.Pending,
		})
		return i.StatusUpdate(ctx, instance)
	}

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

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.Ready,
		Status: metav1.ConditionFalse,
		Reason: constants.Creating,
	})
	return i.StatusUpdate(ctx, instance)

}
