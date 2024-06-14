package actions

import (
	"context"
	"fmt"

	cutils "github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		updated bool
		err     error
	)

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	dp, err := utils.CreateDeployment(instance, DeploymentName, RBACName, labels, ServerTargetPort, MetricsPort)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}
	err = cutils.SetTrustedCA(&dp.Spec.Template, cutils.TrustedCAAnnotationToReference(instance.Annotations))
	if err != nil {
		return i.Error(err)
	}

	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create CTlog: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		i.Recorder.Eventf(instance, v12.EventTypeNormal, "DeploymentUpdated", "Deployment updated: %s", instance.Name)
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) CanHandleError(ctx context.Context, instance *rhtasv1alpha1.CTlog) bool {
	err := i.Client.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, &v1.Deployment{})
	return !meta.IsStatusConditionTrue(instance.GetConditions(), ServerCondition) && err == nil || !errors.IsNotFound(err)
}

func (i deployAction) HandleError(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	redisDeployment := &v1.Deployment{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, redisDeployment); err != nil {
		return i.Error(err)
	}
	if err := i.Client.Delete(ctx, redisDeployment); err != nil {
		i.Logger.V(1).Info("Can't delete CTLog deployment", "error", err.Error())
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "server deployment will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
