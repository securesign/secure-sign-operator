package ui

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled)
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.UIComponentName, actions.SearchUiDeploymentName, instance.Name)
	svc := k8sutils.CreateService(instance.Namespace, actions.SearchUiDeploymentName, actions.SearchUiDeploymentPortName, actions.SearchUiDeploymentPort, actions.SearchUiDeploymentPort, labels)
	svc.Spec.Ports[0].Port = 80

	if err = controllerutil.SetControllerReference(instance, svc, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for service: %w", err))
	}

	if updated, err = i.Ensure(ctx, svc); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i createServiceAction) CanHandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	err := i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, &v1.Service{})
	return utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled) && !meta.IsStatusConditionTrue(instance.GetConditions(), actions.UICondition) && (err == nil || !errors.IsNotFound(err))
}

func (i createServiceAction) HandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	deployment := &v1.Service{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, deployment); err != nil {
		return i.Error(err)
	}
	if err := i.Client.Delete(ctx, deployment); err != nil {
		i.Logger.V(1).Info("Can't delete UI service", "error", err.Error())
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.UICondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "UI service will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
