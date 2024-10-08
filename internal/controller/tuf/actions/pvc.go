package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/core/v1"

	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
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
		updated bool
		err     error
	)

	if instance.Spec.Pvc.Name != "" {
		instance.Status.PvcName = instance.Spec.Pvc.Name
		return i.StatusUpdate(ctx, instance)
	}

	if instance.Spec.Pvc.Size == nil {
		return i.Failed(fmt.Errorf("PVC size is not set"))
	}

	// PVC does not exist, create a new one
	i.Logger.V(1).Info("Creating new PVC")
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.Ready,
		Status: metav1.ConditionFalse,
		Reason: constants.Creating,
	})

	pvc := k8sutils.CreatePVC(instance.Namespace, DeploymentName, rhtasv1alpha1.Pvc{
		Size:         instance.Spec.Pvc.Size,
		AccessModes:  instance.Spec.Pvc.AccessModes,
		StorageClass: instance.Spec.Pvc.StorageClass,
	},
		constants.LabelsFor(DeploymentName, DeploymentName, instance.Name))
	if !utils.OptionalBool(instance.Spec.Pvc.Retain) {
		if err = controllerutil.SetControllerReference(instance, pvc, i.Client.Scheme()); err != nil {
			return i.Failed(fmt.Errorf("could not set controller reference for PVC: %w", err))
		}
	}
	if updated, err = i.Ensure(ctx, pvc); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance)
	}
	if updated {
		i.Recorder.Event(instance, v1.EventTypeNormal, "PersistentVolumeCreated", "New PersistentVolume created")
	}

	instance.Status.PvcName = pvc.Name
	return i.StatusUpdate(ctx, instance)
}
