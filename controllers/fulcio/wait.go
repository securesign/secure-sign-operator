package fulcio

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
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

func (i waitAction) CanHandle(Fulcio *rhtasv1alpha1.Fulcio) bool {
	return Fulcio.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) (*rhtasv1alpha1.Fulcio, error) {
	var (
		ok  bool
		err error
	)
	labels := commonUtils.FilterCommonLabels(instance.Labels)
	labels["app.kubernetes.io/component"] = ComponentName
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	if !ok {
		return instance, nil
	}
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
