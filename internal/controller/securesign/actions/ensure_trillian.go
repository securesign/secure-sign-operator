package actions

import (
	"context"

	"github.com/securesign/operator/internal/controller/annotations"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewTrillianAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &trillianAction{}
}

type trillianAction struct {
	action.BaseAction
}

func (i trillianAction) Name() string {
	return "create trillian"
}

func (i trillianAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i trillianAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	trillian := &rhtasv1alpha1.Trillian{}

	trillian.Name = instance.Name
	trillian.Namespace = instance.Namespace
	trillian.Labels = constants.LabelsFor("trillian", trillian.Name, instance.Name)
	trillian.Annotations = annotations.FilterInheritable(instance.Annotations)

	trillian.Spec = instance.Spec.Trillian

	if err = controllerutil.SetControllerReference(instance, trillian, i.Client.Scheme()); err != nil {
		return i.Error(err)
	}

	if updated, err = i.Ensure(ctx, trillian); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TrillianCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TrillianCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Trillian resource created " + trillian.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(trillian), instance)
}

func (i trillianAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	object := &rhtasv1alpha1.Trillian{}
	if err := i.Client.Get(ctx, ok, object); err != nil {
		return i.Error(err)
	}
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if meta.FindStatusCondition(instance.Status.Conditions, TrillianCondition).Reason != objectStatus.Reason {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   TrillianCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}

func (i trillianAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.Securesign) bool {
	return false
}

func (i trillianAction) HandleError(_ context.Context, _ *rhtasv1alpha1.Securesign) *action.Result {
	return i.Continue()
}
