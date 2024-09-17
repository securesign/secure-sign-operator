package actions

import (
	"context"

	"github.com/securesign/operator/internal/controller/annotations"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewFulcioAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &fulcioAction{}
}

type fulcioAction struct {
	action.BaseAction
}

func (i fulcioAction) Name() string {
	return "create fulcio"
}

func (i fulcioAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i fulcioAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	fulcio := &rhtasv1alpha1.Fulcio{}

	fulcio.Name = instance.Name
	fulcio.Namespace = instance.Namespace
	fulcio.Labels = constants.LabelsFor(actions.ComponentName, fulcio.Name, instance.Name)
	fulcio.Annotations = annotations.FilterInheritable(instance.Annotations)

	fulcio.Spec = instance.Spec.Fulcio

	if err = controllerutil.SetControllerReference(instance, fulcio, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, fulcio, action.EnsureSpec(), action.EnsureRouteSelectorLabels()); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    FulcioCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    FulcioCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Fulcio resource created " + fulcio.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(fulcio), instance)
}

func (i fulcioAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	object := &rhtasv1alpha1.Fulcio{}
	if err := i.Client.Get(ctx, ok, object); err != nil {
		return i.Failed(err)
	}
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, FulcioCondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   FulcioCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		if objectStatus.Status == v1.ConditionTrue {
			instance.Status.FulcioStatus.Url = object.Status.Url
		}
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
