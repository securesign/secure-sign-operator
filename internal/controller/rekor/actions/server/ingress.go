package server

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewIngressAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ingress"
}

func (i ingressAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) &&
		instance.Spec.ExternalAccess.Enabled
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var updated bool
	ok := types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Error(fmt.Errorf("could not find service for ingress: %w", err))
	}

	ingress, err := kubernetes.CreateIngress(ctx, i.Client, *svc, instance.Spec.ExternalAccess, actions.ServerDeploymentPortName, labels)
	if err != nil {
		return i.Error(fmt.Errorf("could not create ingress object: %w", err))
	}

	if err = controllerutil.SetControllerReference(instance, ingress, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for Ingress: %w", err))
	}

	if updated, err = i.Ensure(ctx, ingress); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Ingress created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i ingressAction) CanHandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	err := i.Client.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}, &v12.Ingress{})
	return !meta.IsStatusConditionTrue(instance.GetConditions(), actions.ServerCondition) && instance.Spec.ExternalAccess.Enabled && (err == nil || !errors.IsNotFound(err))
}

func (i ingressAction) HandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	deployment := &v12.Ingress{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}, deployment); err != nil {
		if errors.IsNotFound(err) {
			{
				return i.Continue()
			}
		}
	}
	if err := i.Client.Delete(ctx, deployment); err != nil {
		i.Logger.V(1).Info("Can't delete server ingress", "error", err.Error())
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "server ingress will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
