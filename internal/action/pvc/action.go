package pvc

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewAction[T apis.ConditionsAwareObject](pvcNameFormat, component, deployment string, wrapper func(T) *wrapper[T]) action.Action[T] {
	a := &pvcAction[T]{
		nameFormat: pvcNameFormat,
		component:  component,
		deployment: deployment,
		wrapper:    wrapper,
	}
	return a
}

type pvcAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	nameFormat string
	component  string
	deployment string
	wrapper    func(T) *wrapper[T]
}

func (i pvcAction[T]) Name() string {
	return "PVC"
}

func (i pvcAction[T]) CanHandle(_ context.Context, instance T) bool {
	wrapped := i.wrapper(instance)

	if !wrapped.EnabledPVC() {
		return false
	}

	if wrapped.GetStatusPVCName() == "" {
		return true
	}

	if !meta.IsStatusConditionTrue(instance.GetConditions(), ConditionType) {
		return true
	}

	c := meta.FindStatusCondition(instance.GetConditions(), ConditionType)
	return c.ObservedGeneration != instance.GetGeneration()
}

func (i pvcAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	wrapped := i.wrapper(instance)
	pvcSpec := wrapped.GetPVCSpec()

	if pvcSpec.Name != "" {
		wrapped.SetStatusPVCName(pvcSpec.Name)
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonSpecified,
			ObservedGeneration: instance.GetGeneration(),
		})
		return i.StatusUpdate(ctx, instance)
	}

	var name string
	if strings.Contains(i.nameFormat, "%s") {
		name = fmt.Sprintf(i.nameFormat, instance.GetName())
	} else {
		name = i.nameFormat
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.GetNamespace(),
		},
	}

	// discover existing default PVC and use it
	if wrapped.GetStatusPVCName() == "" && i.Client.Get(ctx, client.ObjectKeyFromObject(pvc), pvc) == nil {
		wrapped.SetStatusPVCName(pvc.GetName())
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonDiscovered,
			Message:            fmt.Sprintf("Discovered and using  existing default PVC `%s`.", pvc.GetName()),
			ObservedGeneration: instance.GetGeneration(),
		})
		return i.StatusUpdate(ctx, instance)
	}

	if pvcSpec.Size == nil {
		return i.Error(ctx, reconcile.TerminalError(ErrPVCSizeNotSet), instance, metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            ErrPVCSizeNotSet.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	l := labels.For(i.component, i.deployment, instance.GetName())
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client, pvc,
		kubernetes.EnsurePVCSpec(pvcSpec),
		ensure.Optional[*v1.PersistentVolumeClaim](!utils.OptionalBool(pvcSpec.Retain), ensure.ControllerReference[*v1.PersistentVolumeClaim](instance, i.Client)),
		ensure.Labels[*v1.PersistentVolumeClaim](slices.Collect(maps.Keys(l)), l),
	); err != nil {
		// do not terminate the deployment - retry with exponential backoff
		return i.Error(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance, metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	switch result {
	case controllerutil.OperationResultCreated:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonCreated,
			ObservedGeneration: instance.GetGeneration(),
		})
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "PersistentVolumeClaimCreated", "New PersistentVolumeClaim created `%s`", pvc.Name)
	case controllerutil.OperationResultUpdated:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonUpdated,
			ObservedGeneration: instance.GetGeneration(),
		})
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "PersistentVolumeClaimUpdated", "PersistentVolumeClaim updated `%s`", pvc.Name)
	case controllerutil.OperationResultNone:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             constants.Ready,
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	wrapped.SetStatusPVCName(pvc.Name)
	return i.StatusUpdate(ctx, instance)
}
