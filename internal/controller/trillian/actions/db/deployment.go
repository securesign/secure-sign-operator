package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/core/v1"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Ready || c.Reason == constants.Creating) && utils.OptionalBool(instance.Spec.Db.Create)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.DbComponentName, actions.DbDeploymentName, instance.Name)
	scc, err := kubernetes.GetOpenshiftPodSecurityContextRestricted(ctx, i.Client, instance.Namespace)
	if err != nil {
		i.Logger.Info("Can't resolve OpenShift scc - using default values", "Error", err.Error(), "Fallback FSGroup", "1001")
		scc = &v1.PodSecurityContext{FSGroup: utils.Pointer(int64(1001)), FSGroupChangePolicy: utils.Pointer(v1.FSGroupChangeOnRootMismatch)}
	}

	db, err := trillianUtils.CreateTrillDb(instance, actions.DbDeploymentName, actions.RBACName, scc, labels)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
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
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance)
	}
	if err = controllerutil.SetControllerReference(instance, db, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for DB Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, db); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
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
		return i.ErrorWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Database deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.Trillian) bool {
	return false
}

func (i deployAction) HandleError(_ context.Context, _ *rhtasv1alpha1.Trillian) *action.Result {
	return i.Continue()
}
