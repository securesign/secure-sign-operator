package redis

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)
	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)
	dp := utils.CreateRedisDeployment(instance.Namespace, actions.RedisDeploymentName, actions.RBACName, labels)
	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Rekor redis: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Redis created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
