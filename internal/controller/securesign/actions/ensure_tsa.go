package actions

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err    error
		result controllerutil.OperationResult
		l      = labels.For(actions.ComponentName, instance.Name, instance.Name)
		tsa    = &rhtasv1alpha1.TimestampAuthority{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if reflect.ValueOf(instance.Spec.TimestampAuthority).IsZero() {
		if meta.IsStatusConditionTrue(instance.Status.Conditions, TSACondition) {
			return i.Continue()
		}
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TSACondition,
			Status:  v1.ConditionTrue,
			Reason:  constants.NotDefined,
			Message: "TSA resource is undefined",
		})
		return i.StatusUpdate(ctx, instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		tsa,
		ensure.ControllerReference[*rhtasv1alpha1.TimestampAuthority](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.TimestampAuthority](slices.Collect(maps.Keys(l)), l),
		ensure.Annotations[*rhtasv1alpha1.TimestampAuthority](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.TimestampAuthority) error {
			object.Spec = *instance.Spec.TimestampAuthority
			object.Spec.ImagePullSecrets = utils.MergeImagePullSecrets(
				instance.Spec.ImagePullSecrets,
				instance.Spec.TimestampAuthority.ImagePullSecrets,
			)
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TimestampAuthority: %w", err), instance,
			v1.Condition{
				Type:    TSACondition,
				Status:  v1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    TSACondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "TSA resource created " + tsa.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, tsa, instance)
}

func (i tsaAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.TimestampAuthority, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	switch {
	case !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, TSACondition, objectStatus.Status):
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   TSACondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
	case instance.Status.TSAStatus.Url != object.Status.Url:
		instance.Status.TSAStatus.Url = object.Status.Url
	default:
		return i.Continue()
	}

	return i.StatusUpdate(ctx, instance)
}
