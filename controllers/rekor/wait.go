package rekor

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	commonUtils "github.com/securesign/operator/controllers/common/utils"
)

func NewWaitAction() Action {
	return &waitAction{}
}

type waitAction struct {
	common.BaseAction
}

func (i waitAction) Name() string {
	return "wait"
}

func (i waitAction) CanHandle(Rekor *rhtasv1alpha1.Rekor) bool {
	return Rekor.Status.Phase == rhtasv1alpha1.PhaseInitialization
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
	var (
		ok  bool
		err error
	)
	for _, deployment := range []string{"rekor-server", "rekor-redis"} {
		ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, deployment)
		if err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, err
		}
		if !ok {
			return instance, nil
		}
	}
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
