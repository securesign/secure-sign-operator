package tuf

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	fulcioUtils "github.com/securesign/operator/controllers/fulcio/utils"
	rekorUtils "github.com/securesign/operator/controllers/rekor/utils"
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

func (i pendingAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseNone || tuf.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	if instance.Status.Phase == rhtasv1alpha1.PhaseNone {
		instance.Status.Phase = rhtasv1alpha1.PhasePending
	}

	rekor, err := rekorUtils.FindRekor(ctx, i.Client, instance.Namespace, kubernetes.FilterCommonLabels(instance.Labels))
	if err != nil || rekor.Status.Phase != rhtasv1alpha1.PhaseReady {
		i.Logger.V(1).Info("Rekor is not ready")
		return instance, nil
	}

	fulcio, err := fulcioUtils.FindFulcio(ctx, i.Client, instance.Namespace, kubernetes.FilterCommonLabels(instance.Labels))
	if err != nil || fulcio.Status.Phase != rhtasv1alpha1.PhaseReady {
		i.Logger.V(1).Info("Fulcio is not ready")
		return instance, nil
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, err
}
