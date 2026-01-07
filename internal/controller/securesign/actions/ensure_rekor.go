package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err    error
		result controllerutil.OperationResult
		l      = labels.For("rekor", instance.Name, instance.Name)
		rekor  = &rhtasv1alpha1.Rekor{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		rekor,
		ensure.ControllerReference[*rhtasv1alpha1.Rekor](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.Rekor](slices.Collect(maps.Keys(l)), l),
		ensure.Annotations[*rhtasv1alpha1.Rekor](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.Rekor) error {
			object.Spec = instance.Spec.Rekor
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Rekor: %w", err), instance,
			v1.Condition{
				Type:    RekorCondition,
				Status:  v1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    RekorCondition,
			Status:  v1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Rekor resource created " + rekor.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, rekor, instance)
}

func (i rekorAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.Rekor, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.ReadyCondition)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	switch {
	case !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, RekorCondition, objectStatus.Status):
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   RekorCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
	case instance.Status.RekorStatus.Url != object.Status.Url:
		instance.Status.RekorStatus.Url = object.Status.Url
	default:
		return i.Continue()
	}

	return i.StatusUpdate(ctx, instance)
}
