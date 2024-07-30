package actions

import (
	"context"
	"fmt"

	"github.com/google/trillian"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	trillian2 "github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
)

type createTree func(ctx context.Context, displayName string, trillianURL string, deadline int64) (*trillian.Tree, error)

func NewResolveTreeAction(opts ...func(*resolveTreeAction)) action.Action[*rhtasv1alpha1.CTlog] {
	a := &resolveTreeAction{
		createTree: common.CreateTrillianTree,
	}

	for _, opt := range opts {
		opt(a)
	}
	return a
}

type resolveTreeAction struct {
	action.BaseAction
	createTree createTree
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
	trillUrl := fmt.Sprintf("%s.%s.svc:8091", trillian2.LogserverDeploymentName, instance.Namespace)
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	tree, err = i.createTree(ctx, "ctlog-tree", trillUrl, constants.CreateTreeDeadline)
	if err != nil {
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %v", err), instance)
	}
	i.Recorder.Eventf(instance, v1.EventTypeNormal, "TrillianTreeCreated", "New Trillian tree created: %d", tree.TreeId)
	instance.Status.TreeID = &tree.TreeId

	return i.StatusUpdate(ctx, instance)
}

func (i resolveTreeAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.CTlog) bool {
	// instance.Status.TreeID == nil in case of failure
	// no need to recover
	return false
}

func (i resolveTreeAction) HandleError(_ context.Context, _ *rhtasv1alpha1.CTlog) *action.Result {
	return i.Continue()
}
