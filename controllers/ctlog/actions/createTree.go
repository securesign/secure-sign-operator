package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (i createTrillianTreeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating && (instance.Spec.TreeID == nil || *instance.Spec.TreeID == int64(0))
}

func (i createTrillianTreeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var err error

	trillUrl, err := utils.GetInternalUrl(ctx, i.Client, instance.Namespace, trillian.LogserverDeploymentName)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not find trillian instance: %w", err), instance)
	}
	tree, err := common.CreateTrillianTree(ctx, "ctlog-tree", trillUrl+":8091")
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %w", err), instance)
	}
	i.Recorder.Event(instance, v1.EventTypeNormal, "TreeID", "New Trillian tree created")
	instance.Spec.TreeID = &tree.TreeId

	return i.Update(ctx, instance)
}
