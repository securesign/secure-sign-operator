package actions

import (
	"context"
	"fmt"

	cutils "github.com/securesign/operator/internal/controller/common/utils"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
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

	switch {
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	dp, err := utils.CreateDeployment(instance, DeploymentName, RBACName, labels, ServerTargetPort, MetricsPort)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}
	err = cutils.SetTrustedCA(&dp.Spec.Template, cutils.TrustedCAAnnotationToReference(instance.Annotations))
	if err != nil {
		return i.Failed(err)
	}

	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create CTlog: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "Service created",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
