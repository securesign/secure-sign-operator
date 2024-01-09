package ctlog

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

func (i pendingAction) CanHandle(tuf *rhtasv1alpha1.CTlog) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseNone || tuf.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) (*rhtasv1alpha1.CTlog, error) {
	if instance.Status.Phase == rhtasv1alpha1.PhaseNone {
		instance.Status.Phase = rhtasv1alpha1.PhasePending
	}

	trillians, err := findTrillians(ctx, i.Client, *instance)
	if err != nil {
		return instance, err
	}
	if len(trillians.Items) == 0 || trillians.Items[0].Status.Phase != rhtasv1alpha1.PhaseReady {
		i.Logger.V(1).Info("waiting for trillian")
		return instance, nil
	}

	fulcios, err := findFulcios(ctx, i.Client, *instance)
	if err != nil {
		return instance, err
	}
	if len(fulcios.Items) == 0 || fulcios.Items[0].Status.Phase != rhtasv1alpha1.PhaseReady {
		i.Logger.V(1).Info("waiting for fulcio")
		return instance, nil
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, err
}

func findTrillians(ctx context.Context, cli client.Client, instance rhtasv1alpha1.CTlog) (*rhtasv1alpha1.TrillianList, error) {
	searchLabels := kubernetes.FilterCommonLabels(instance.Labels)

	list := &rhtasv1alpha1.TrillianList{}
	err := cli.List(ctx, list, client.InNamespace(instance.Namespace), client.MatchingLabels(searchLabels))
	if err != nil {
		return nil, err
	}
	return list, nil
}

func findFulcios(ctx context.Context, cli client.Client, instance rhtasv1alpha1.CTlog) (*rhtasv1alpha1.FulcioList, error) {
	searchLabels := kubernetes.FilterCommonLabels(instance.Labels)

	list := &rhtasv1alpha1.FulcioList{}
	err := cli.List(ctx, list, client.InNamespace(instance.Namespace), client.MatchingLabels(searchLabels))
	if err != nil {
		return nil, err
	}
	return list, nil
}
