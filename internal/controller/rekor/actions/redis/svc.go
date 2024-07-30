package redis

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
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

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)
	svc := k8sutils.CreateService(instance.Namespace, actions.RedisDeploymentName, actions.RedisDeploymentPortName, actions.RedisDeploymentPort, actions.RedisDeploymentPort, labels)

	if err = controllerutil.SetControllerReference(instance, svc, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for Redis service: %w", err))
	}

	if updated, err = i.Ensure(ctx, svc); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
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
	err := i.Client.Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: instance.Namespace}, &v1.Service{})
	return !meta.IsStatusConditionTrue(instance.GetConditions(), actions.RedisCondition) && (err == nil || !errors.IsNotFound(err))
}

func (i createServiceAction) HandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	svc := &v1.Service{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: instance.Namespace}, svc); err != nil {
		return i.Error(err)
	}
	if err := i.Client.Delete(ctx, svc); err != nil {
		i.Logger.V(1).Info("Can't delete Redis service", "error", err.Error())
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.RedisCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "Redis service will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
