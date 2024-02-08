package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
)

func NewPendingAction() action.Action[rhtasv1alpha1.Rekor] {
	return &pendingAction{}
}

type pendingAction struct {
	action.BaseAction
}

func (i pendingAction) Name() string {
	return "pending"
}

func (i pendingAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseNone || instance.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	if instance.Status.Phase == rhtasv1alpha1.PhaseNone {
		instance.Status.Phase = rhtasv1alpha1.PhasePending
		return i.StatusUpdate(ctx, instance)
	}

	var err error
	_, err = utils.GetInternalUrl(ctx, i.Client, instance.Namespace, trillian.LogserverDeploymentName)
	if err != nil {
		//TODO: add status condition - waiting for trillian
		return i.Requeue()
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return i.StatusUpdate(ctx, instance)

}
