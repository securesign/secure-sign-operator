package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err      error
		result   controllerutil.OperationResult
		l        = labels.For("trillian", instance.Name, instance.Name)
		trillian = &rhtasv1alpha1.Trillian{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		trillian,
		ensure.ControllerReference[*rhtasv1alpha1.Trillian](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.Trillian](slices.Collect(maps.Keys(l)), l),
		ensure.Annotations[*rhtasv1alpha1.Trillian](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.Trillian) error {
			object.Spec = instance.Spec.Trillian
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian: %w", err), instance,
			v1.Condition{
				Type:    TrillianCondition,
				Status:  v1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TrillianCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Trillian resource created " + trillian.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, trillian, instance)
}

func (i trillianAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.Trillian, instance *rhtasv1alpha1.Securesign) *action.Result {
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
