package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type deployAction struct {
	action.BaseAction
}

func NewDeployAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &deployAction{}
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	if instance.Spec.Signer.CertificateChain.CertificateChainRef == nil &&
		(instance.Spec.Signer.CertificateChain.RootCA == nil ||
			instance.Spec.Signer.CertificateChain.LeafCA == nil) {
		return false
	}

	return (c.Reason == constants.Ready || c.Reason == constants.Creating)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	var (
		updated bool
		err     error
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)
	deployment, err := tsaUtils.CreateTimestampAuthorityDeployment(instance, DeploymentName, RBACName, labels)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
	}
	if err = controllerutil.SetControllerReference(instance, deployment, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, deployment); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create TSA Server: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "TSA server deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
