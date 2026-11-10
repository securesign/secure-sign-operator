package pvc

import (
	"context"
	"fmt"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/util/csaupgrade"
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

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.GetNamespace(),
		},
	}

	exists := false
	if getErr := i.Client.Get(ctx, client.ObjectKeyFromObject(pvc), pvc); getErr == nil {
		exists = true
	} else if !apierrors.IsNotFound(getErr) {
		return i.Error(ctx, fmt.Errorf("could not get PVC: %w", getErr), instance)
	}

	// discover existing default PVC and use it
	if wrapped.GetStatusPVCName() == "" && exists {
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

	spec := corev1ac.PersistentVolumeClaimSpec().WithResources(
		corev1ac.VolumeResourceRequirements().WithRequests(corev1.ResourceList{
			corev1.ResourceStorage: pvcSpec.Size.DeepCopy(),
		}),
	)
	for _, mode := range pvcSpec.AccessModes {
		spec.WithAccessModes(corev1.PersistentVolumeAccessMode(mode))
	}
	if pvcSpec.StorageClass != "" {
		spec.WithStorageClassName(pvcSpec.StorageClass)
	}

	desired, err := traits.Apply(
		corev1ac.PersistentVolumeClaim(name, instance.GetNamespace()).WithSpec(spec),
		traits.Optional(exists, i.RestoreImmutableFields(pvc, pvcSpec)),
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
		i.Recorder.Eventf(instance, pvc, corev1.EventTypeNormal, "PersistentVolumeClaimCreated", "Created", "New PersistentVolumeClaim created `%s`", pvc.Name)
	case changed:
		instance.SetCondition(metav1.Condition{
			Type:               ConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonUpdated,
			ObservedGeneration: instance.GetGeneration(),
		})
		i.Recorder.Eventf(instance, pvc, corev1.EventTypeNormal, "PersistentVolumeClaimUpdated", "Updated", "PersistentVolumeClaim updated `%s`", pvc.Name)
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

// WORKAROUND: Use rhtasv1.Pvc values for storageClassName and accessModes only when
// creating a PVC (immutable fields on corev1.PersistentVolumeClaim); keep them unchanged on existing PVCs. These fields should
// be immutable in the CR API, but OLMv1's CRD upgrade safety check rejects
// the CEL validation changes needed to enforce this.
func (i *pvcAction[T]) RestoreImmutableFields(existing *corev1.PersistentVolumeClaim, pvc rhtasv1.Pvc) traits.Trait[*corev1ac.PersistentVolumeClaimApplyConfiguration] {
	return func(desired *corev1ac.PersistentVolumeClaimApplyConfiguration) error {
		i.Logger.V(1).Info("upgrading existing PVC - immutable fields will be kept unchanged", "pvc", existing.GetName())
		var fieldManager string
		if withManager, ok := i.Client.(*kubernetes.ManagedClient); !ok {
			return fmt.Errorf("can't derive field manager from client")
		} else {
			fieldManager = withManager.GetManager()
		}
		if err := csaupgrade.UpgradeManagedFields(existing, sets.New(kubernetes.LegacyFieldManager), fieldManager); err != nil {
			return fmt.Errorf("could not upgrade managed fields: %w", err)
		}
		pvcAc, err := corev1ac.ExtractPersistentVolumeClaim(existing, fieldManager)
		if err != nil {
			return fmt.Errorf("could not extract PVC: %w", err)
		}

		if pvcAc.Spec != nil {
			desired.Spec.AccessModes = pvcAc.Spec.AccessModes
			desired.Spec.StorageClassName = pvcAc.Spec.StorageClassName
		} else {
			desired.Spec.AccessModes = nil
			desired.Spec.StorageClassName = nil
		}
		return nil
	}
}
