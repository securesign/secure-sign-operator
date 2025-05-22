package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreatePvcAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &createPvcAction{}
}

type createPvcAction struct {
	action.BaseAction
}

func (i createPvcAction) Name() string {
	return "create PVC"
}

func (i createPvcAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && instance.Status.PvcName == ""
}

func (i createPvcAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	if instance.Spec.Pvc.Name != "" {
		instance.Status.PvcName = instance.Spec.Pvc.Name
		return i.StatusUpdate(ctx, instance)
	}

	if instance.Spec.Pvc.Size == nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("PVC size is not set")), instance)
	}

	// PVC does not exist, create a new one
	i.Logger.V(1).Info("Creating new PVC")
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.Ready,
		Status: metav1.ConditionFalse,
		Reason: constants.Creating,
	})

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.DeploymentName,
			Namespace: instance.Namespace,
		},
	}

	l := labels.For(tufConstants.DeploymentName, tufConstants.DeploymentName, instance.Name)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client, pvc,
		kubernetes.EnsurePVCSpec(rhtasv1alpha1.Pvc{
			Size:         instance.Spec.Pvc.Size,
			AccessModes:  instance.Spec.Pvc.AccessModes,
			StorageClass: instance.Spec.Pvc.StorageClass,
		}),
		ensure.Optional[*v1.PersistentVolumeClaim](!utils.OptionalBool(instance.Spec.Pvc.Retain), ensure.ControllerReference[*v1.PersistentVolumeClaim](instance, i.Client)),
		ensure.Labels[*v1.PersistentVolumeClaim](slices.Collect(maps.Keys(l)), l),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		i.Recorder.Event(instance, v1.EventTypeNormal, "PersistentVolumeCreated", "New PersistentVolume created")
	}

	instance.Status.PvcName = pvc.Name
	return i.StatusUpdate(ctx, instance)
}
