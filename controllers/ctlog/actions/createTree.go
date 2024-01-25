package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	v1 "k8s.io/api/core/v1"
)

func NewCreateTrillianTreeAction() action.Action[rhtasv1alpha1.CTlog] {
	return &createTrillianTreeAction{}
}

type createTrillianTreeAction struct {
	action.BaseAction
}

func (i createTrillianTreeAction) Name() string {
	return "create Trillian tree"
}

func (i createTrillianTreeAction) CanHandle(instance *rhtasv1alpha1.CTlog) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating && (instance.Spec.TreeID == nil || *instance.Spec.TreeID == int64(0))
}

func (i createTrillianTreeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var err error

	trillUrl, err := utils.GetInternalUrl(ctx, i.Client, instance.Namespace, trillian.LogserverDeploymentName)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not find trillian instance: %w", err), instance)
	}
	tree, err := common.CreateTrillianTree(ctx, "ctlog-tree", trillUrl+":8091")
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %w", err), instance)
	}
	i.Recorder.Event(instance, v1.EventTypeNormal, "TreeID", "New Trillian tree created")
	instance.Spec.TreeID = &tree.TreeId

	return i.Update(ctx, instance)
}
