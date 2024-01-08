package tuf

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	searchLabels := kubernetes.FilterCommonLabels(instance.Labels)

	rekorList := &rhtasv1alpha1.RekorList{}
	err := i.Client.List(ctx, rekorList, client.InNamespace(instance.Namespace), client.MatchingLabels(searchLabels))
	if err != nil {
		return instance, err
	}
	if len(rekorList.Items) == 0 || rekorList.Items[0].Status.Phase != rhtasv1alpha1.PhaseReady {
		return instance, nil
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, err
}
