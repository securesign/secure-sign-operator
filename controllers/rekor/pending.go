package rekor

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
)

func NewPendingAction() Action {
	return &pendingAction{}
}

type pendingAction struct {
	common.BaseAction
}

func (i pendingAction) Name() string {
	return "pending"
}

func (i pendingAction) CanHandle(tuf *rhtasv1alpha1.Rekor) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseNone || tuf.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
	if instance.Status.Phase == rhtasv1alpha1.PhaseNone {
		instance.Status.Phase = rhtasv1alpha1.PhasePending
	}

	trillian, err := trillianUtils.FindTrillian(ctx, i.Client, instance.Namespace, kubernetes.FilterCommonLabels(instance.Labels))
	if err != nil || trillian.Status.Phase != rhtasv1alpha1.PhaseReady {
		i.Logger.V(1).Info("Trillian is not ready")
		return instance, nil
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, err
}
