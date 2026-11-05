package pvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
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
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	if pvcSpec.Size == nil {
		return i.Error(ctx, reconcile.TerminalError(ErrPVCSizeNotSet), instance, metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            ErrPVCSizeNotSet.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	desired, err := traits.Apply(corev1ac.PersistentVolumeClaim(name, instance.GetNamespace()),
		traits.PVCSpec(pvcSpec),
		traits.Optional(!utils.OptionalBool(pvcSpec.Retain), traits.ControllerReference[*corev1ac.PersistentVolumeClaimApplyConfiguration](instance, i.Client)),
		traits.Labels[*corev1ac.PersistentVolumeClaimApplyConfiguration](labels.For(i.component, i.deployment, instance.GetName())),
	)
	var changed bool
	if err == nil {
		changed, err = kubernetes.Apply(ctx, i.Client, desired, pvc)
	}
	if err != nil {
		// do not terminate the deployment - retry with exponential backoff
		return i.Error(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance, metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	switch {
	case changed && pvc.ResourceVersion == "":
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonCreated,
			ObservedGeneration: instance.GetGeneration(),
		})
		i.Recorder.Eventf(instance, pvc, v1.EventTypeNormal, "PersistentVolumeClaimCreated", "Created", "New PersistentVolumeClaim created `%s`", pvc.Name)
	case changed:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonUpdated,
			ObservedGeneration: instance.GetGeneration(),
		})
		i.Recorder.Eventf(instance, pvc, v1.EventTypeNormal, "PersistentVolumeClaimUpdated", "Updated", "PersistentVolumeClaim updated `%s`", pvc.Name)
	default:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             state.Ready.String(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	wrapped.SetStatusPVCName(pvc.Name)
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
