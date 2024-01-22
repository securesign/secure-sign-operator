package trillian

import (
	"context"
	"github.com/securesign/operator/controllers/common/action"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
)

func NewWaitAction() action.Action[rhtasv1alpha1.Trillian] {
	return &waitAction{}
}

type waitAction struct {
	action.BaseAction
}

func (i waitAction) Name() string {
	return "wait"
}

func (i waitAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	var (
		ok  bool
		err error
	)
	labels := commonUtils.FilterCommonLabels(instance.Labels)
	labels[commonUtils.ComponentLabel] = ComponentName
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	if !ok {
		return instance, nil
	}
	instance.Status.Phase = rhtasv1alpha1.PhaseInitialize
	return instance, nil
}
