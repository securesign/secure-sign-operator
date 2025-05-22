package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err    error
		result controllerutil.OperationResult
		l      = labels.For(tufConstants.ComponentName, instance.Name, instance.Name)
		tuf    = &rhtasv1alpha1.Tuf{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		tuf,
		ensure.ControllerReference[*rhtasv1alpha1.Tuf](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.Tuf](slices.Collect(maps.Keys(l)), l),
		ensure.Annotations[*rhtasv1alpha1.Tuf](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.Tuf) error {
			object.Spec = instance.Spec.Tuf
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Tuf: %w", err), instance,
			v1.Condition{
				Type:    TufCondition,
				Status:  v1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TufCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Tuf resource created " + tuf.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, tuf, instance)
}

func (i tufAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.Tuf, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}

	switch {
	case !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, TufCondition, objectStatus.Status):
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   TufCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
	case instance.Status.TufStatus.Url != object.Status.Url:
		instance.Status.TufStatus.Url = object.Status.Url
	default:
		return i.Continue()
	}

	return i.StatusUpdate(ctx, instance)
}
