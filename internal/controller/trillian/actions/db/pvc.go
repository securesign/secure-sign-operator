package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/labels"
	"golang.org/x/exp/maps"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreatePvcAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &createPvcAction{}
}

type createPvcAction struct {
	action.BaseAction
}

func (i createPvcAction) Name() string {
	return "create PVC"
}

func (i createPvcAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating && utils.OptionalBool(instance.Spec.Db.Create) && instance.Status.Db.Pvc.Name == ""
}

func (i createPvcAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	if instance.Spec.Db.Pvc.Name != "" {
		instance.Status.Db.Pvc.Name = instance.Spec.Db.Pvc.Name
		return i.StatusUpdate(ctx, instance)
	}

	if instance.Spec.Db.Pvc.Size == nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("PVC size is not set")), instance)
	}

	// PVC does not exist, create a new one
	i.Logger.V(1).Info("Creating new PVC")

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      actions.DbPvcName,
			Namespace: instance.Namespace,
		},
	}

	l := labels.For(actions.DbComponentName, actions.DbDeploymentName, instance.Name)
	if result, err = k8sutils.CreateOrUpdate(ctx, i.Client, pvc,
		k8sutils.EnsurePVCSpec(instance.Spec.Db.Pvc),
		ensure.Optional[*v1.PersistentVolumeClaim](!utils.OptionalBool(instance.Spec.Db.Pvc.Retain), ensure.ControllerReference[*v1.PersistentVolumeClaim](instance, i.Client)),
		ensure.Labels[*v1.PersistentVolumeClaim](maps.Keys(l), l),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		i.Recorder.Event(instance, v1.EventTypeNormal, "PersistentVolumeCreated", "New PersistentVolume created")
	}

	instance.Status.Db.Pvc.Name = pvc.Name
	return i.StatusUpdate(ctx, instance)
}
