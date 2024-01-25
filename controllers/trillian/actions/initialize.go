package actions2

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
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

func (i initializeAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	labels := constants.LabelsForComponent(ComponentName, instance.Name)
	ok, err := commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		// deployment is watched - no need to requeue
		return i.Return()
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return i.StatusUpdate(ctx, instance)
}
