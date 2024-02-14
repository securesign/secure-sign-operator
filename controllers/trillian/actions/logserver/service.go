package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[rhtasv1alpha1.Trillian] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)
	logserverService := k8sutils.CreateService(instance.Namespace, actions.LogserverDeploymentName, serverPort, labels)

	if err = controllerutil.SetControllerReference(instance, logserverService, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for logserver Service: %w", err))
	}

	if updated, err = i.Ensure(ctx, logserverService); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create logserver Service: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
