package actions

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/trillian"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewResolveTreeAction(opts ...func(*resolveTreeAction)) action.Action[*rhtasv1alpha1.CTlog] {
	a := &resolveTreeAction{}

	for _, opt := range opts {
		opt(a)
	}
	return a
}

type resolveTreeAction struct {
	action.BaseAction
}

func (i resolveTreeAction) Name() string {
	return "resolve treeID"
}

func (i resolveTreeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case instance.Status.TreeID == nil:
		return true
	case instance.Spec.TreeID != nil:
		return !equality.Semantic.DeepEqual(instance.Spec.TreeID, instance.Status.TreeID)
	default:
		return false
	}
}

func (i resolveTreeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	if instance.Spec.TreeID != nil && *instance.Spec.TreeID != int64(0) {
		instance.Status.TreeID = instance.Spec.TreeID
		return i.StatusUpdate(ctx, instance)
	}
	var err error
	var tree *trillian.Tree

	cm := &v1.ConfigMap{}
	err = i.Client.Get(ctx, types.NamespacedName{Name: "ctlog-tree-id-config", Namespace: instance.Namespace}, cm)
	if err != nil || cm.Data == nil {
		i.Logger.Info("ConfigMap not ready or data is empty, requeuing reconciliation")
		return i.Requeue()
	}

	treeId, exists := cm.Data["tree_id"]
	if !exists {
		err = fmt.Errorf("ConfigMap missing tree_id")
		i.Logger.V(1).Error(err, err.Error())
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %v", err), instance)
	}
	treeIdInt, err := strconv.ParseInt(treeId, 10, 64)
	if err != nil {
		i.Logger.V(1).Error(err, err.Error())
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %v", err), instance)
	}
	tree = &trillian.Tree{TreeId: treeIdInt}
	i.Recorder.Eventf(instance, v1.EventTypeNormal, "TrillianTreeCreated", "New Trillian tree created: %d", tree.TreeId)
	instance.Status.TreeID = &tree.TreeId

	return i.StatusUpdate(ctx, instance)
}
