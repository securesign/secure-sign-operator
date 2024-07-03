package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewRekorAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &rekorAction{}
}

type rekorAction struct {
	action.BaseAction
}

func (i rekorAction) Name() string {
	return "create rekor"
}

func (i rekorAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i rekorAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	rekor := &rhtasv1alpha1.Rekor{}

	rekor.Name = instance.Name
	rekor.Namespace = instance.Namespace
	rekor.Labels = constants.LabelsFor("rekor", rekor.Name, instance.Name)

	rekor.Spec = instance.Spec.Rekor

	if err = controllerutil.SetControllerReference(instance, rekor, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, rekor); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    RekorCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    RekorCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Rekor resource created " + rekor.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(rekor), instance)
}

func (i rekorAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	object := &rhtasv1alpha1.Rekor{}
	if err := i.Client.Get(ctx, ok, object); err != nil {
		return i.Failed(err)
	}
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, RekorCondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   RekorCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		if objectStatus.Status == v1.ConditionTrue {
			instance.Status.RekorStatus.Url = object.Status.Url
		}
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
