package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/internal/controller/labels"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err    error
		result controllerutil.OperationResult
		l      = labels.For(actions.ComponentName, instance.Name, instance.Name)
		fulcio = &rhtasv1alpha1.Fulcio{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		fulcio,
		ensure.ControllerReference[*rhtasv1alpha1.Fulcio](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.Fulcio](maps.Keys(l), l),
		ensure.Annotations[*rhtasv1alpha1.Fulcio](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.Fulcio) error {
			object.Spec = instance.Spec.Fulcio
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance,
			v1.Condition{
				Type:    FulcioCondition,
				Status:  v1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    FulcioCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Fulcio resource created " + fulcio.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, fulcio, instance)
}

func (i fulcioAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.Fulcio, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	switch {
	case !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, FulcioCondition, objectStatus.Status):
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   FulcioCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
	case instance.Status.FulcioStatus.Url != object.Status.Url:
		instance.Status.FulcioStatus.Url = object.Status.Url
	default:
		return i.Continue()
	}

	return i.StatusUpdate(ctx, instance)
}
