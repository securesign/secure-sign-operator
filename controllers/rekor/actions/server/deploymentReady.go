package server

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
)

func NewDeploymentReadyAction() action.Action[rhtasv1alpha1.Rekor] {
	return &deploymentReadyAction{}
}

type deploymentReadyAction struct {
	action.BaseAction
}

func (i deploymentReadyAction) Name() string {
	return "deployment ready"
}

func (i deploymentReadyAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i deploymentReadyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := constants.LabelsForComponent(actions.ServerComponentName, instance.Name)
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		// deployment is watched - no need to requeue
		return i.Return()
	}
	return i.Continue()
}
