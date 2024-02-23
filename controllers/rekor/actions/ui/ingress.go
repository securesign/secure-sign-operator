package ui

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewIngressAction() action.Action[rhtasv1alpha1.Rekor] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ingress"
}

func (i ingressAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) &&
		instance.Spec.RekorSearchUI.Enabled
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var updated bool
	ok := types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}
	labels := constants.LabelsFor(actions.UIComponentName, actions.SearchUiDeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Failed(fmt.Errorf("could not find service for ingress: %w", err))
	}

	ingress, err := kubernetes.CreateIngress(ctx, i.Client, *svc, rhtasv1alpha1.ExternalAccess{}, actions.SearchUiDeploymentName, labels)
	if err != nil {
		return i.Failed(fmt.Errorf("could not create ingress object: %w", err))
	}

	if err = controllerutil.SetControllerReference(instance, ingress, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Ingress: %w", err))
	}

	if updated, err = i.Ensure(ctx, ingress); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
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
		return i.Failed(fmt.Errorf("could not create Ingress: %w", err))
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Ingress created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
