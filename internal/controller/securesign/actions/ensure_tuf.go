package actions

import (
	"context"

	"github.com/securesign/operator/internal/controller/annotations"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tuf/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewTufAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &tufAction{}
}

type tufAction struct {
	action.BaseAction
}

func (i tufAction) Name() string {
	return "create tuf"
}

func (i tufAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i tufAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	tuf := &rhtasv1alpha1.Tuf{}

	tuf.Name = instance.Name
	tuf.Namespace = instance.Namespace
	tuf.Labels = constants.LabelsFor(actions.ComponentName, tuf.Name, instance.Name)
	tuf.Annotations = annotations.FilterInheritable(instance.Annotations)

	tuf.Spec = instance.Spec.Tuf

	if err = controllerutil.SetControllerReference(instance, tuf, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, tuf, action.EnsureSpec(), action.EnsureRouteSelectorLabels()); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TufCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TufCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Tuf resource created " + tuf.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(tuf), instance)
}

func (i tufAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	object := &rhtasv1alpha1.Tuf{}
	if err := i.Client.Get(ctx, ok, object); err != nil {
		return i.Failed(err)
	}
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, TufCondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   TufCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		if objectStatus.Status == v1.ConditionTrue {
			instance.Status.TufStatus.Url = object.Status.Url
		}
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
