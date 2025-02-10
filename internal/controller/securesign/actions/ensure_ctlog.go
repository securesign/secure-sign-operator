package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"golang.org/x/exp/maps"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/controller/labels"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCtlogAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &ctlogAction{}
}

type ctlogAction struct {
	action.BaseAction
}

func (i ctlogAction) Name() string {
	return "create ctlog"
}

func (i ctlogAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i ctlogAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
		l      = labels.For(actions.ComponentName, instance.Name, instance.Name)
		ctl    = &rhtasv1alpha1.CTlog{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		ctl,
		ensure.ControllerReference[*rhtasv1alpha1.CTlog](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.CTlog](maps.Keys(l), l),
		ensure.Annotations[*rhtasv1alpha1.CTlog](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.CTlog) error {
			object.Spec = instance.Spec.Ctlog
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Ctlog: %w", err), instance,
			v1.Condition{
				Type:    CTlogCondition,
				Status:  v1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    CTlogCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "CTLog resource updated " + ctl.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, ctl, instance)
}

func (i ctlogAction) CopyStatus(ctx context.Context, ctl *rhtasv1alpha1.CTlog, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(ctl.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, CTlogCondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   CTlogCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
