package ui

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
	commonutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/utils"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && commonutils.IsEnabled(instance.Spec.RekorSearchUI.Enabled)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)
	labels := constants.LabelsFor(actions.UIComponentName, actions.SearchUiDeploymentName, instance.Name)
	dp := utils.CreateRekorSearchUiDeployment(instance, actions.SearchUiDeploymentName, actions.RBACName, labels)
	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create Rekor search UI: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) CanHandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	err := i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, &v1.Deployment{})
	return commonutils.IsEnabled(instance.Spec.RekorSearchUI.Enabled) && !meta.IsStatusConditionTrue(instance.GetConditions(), actions.UICondition) && (err == nil || !errors.IsNotFound(err))
}

func (i deployAction) HandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	deployment := &v1.Deployment{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, deployment); err != nil {
		return i.Error(err)
	}
	if err := i.Client.Delete(ctx, deployment); err != nil {
		i.Logger.V(1).Info("Can't delete UI deployment", "error", err.Error())
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.UICondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "UI deployment will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
