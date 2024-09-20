package actions

import (
	"context"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewTsaAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &tsaAction{}
}

type tsaAction struct {
	action.BaseAction
}

func (i tsaAction) Name() string {
	return "create tsa"
}

func (i tsaAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i tsaAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	tsa := &rhtasv1alpha1.TimestampAuthority{}
	tsa.Name = instance.Name
	tsa.Namespace = instance.Namespace
	tsa.Labels = constants.LabelsFor(actions.ComponentName, tsa.Name, instance.Name)

	if reflect.ValueOf(instance.Spec.TimestampAuthority).IsZero() {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TSACondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.NotDefined,
			Message: "TSA resource is undefined",
		})
		return i.StatusUpdate(ctx, instance)
	}
	tsa.Spec = *instance.Spec.TimestampAuthority

	if err = controllerutil.SetControllerReference(instance, tsa, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, tsa, action.EnsureSpec(), action.EnsureRouteSelectorLabels()); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TSACondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TSACondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "TSA resource created " + tsa.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(tsa), instance)
}

func (i tsaAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	object := &rhtasv1alpha1.TimestampAuthority{}
	if err := i.Client.Get(ctx, ok, object); err != nil {
		return i.Failed(err)
	}
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, TSACondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   TSACondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		if objectStatus.Status == v1.ConditionTrue {
			instance.Status.TSAStatus.Url = object.Status.Url
		}
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
