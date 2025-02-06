package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"golang.org/x/exp/maps"
	v2 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewInitJobAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &initJobAction{}
}

type initJobAction struct {
	action.BaseAction
}

func (i initJobAction) Name() string {
	return "tuf-init job"
}

func (i initJobAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return c.Reason == constants.Creating && !meta.IsStatusConditionTrue(instance.GetConditions(), tufConstants.RepositoryCondition)
}

func (i initJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	if instance.Spec.Pvc.Name != "" {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    tufConstants.RepositoryCondition,
			Status:  metav1.ConditionTrue,
			Reason:  constants.Ready,
			Message: "Using self-managed tuf repository.",
		})
		return i.StatusUpdate(ctx, instance)
	}

	if j, err := job.GetJob(ctx, i.Client, instance.Namespace, tufConstants.InitJobName); j != nil {
		i.Logger.Info("Tuf tuf-repository-init is already present.", "Succeeded", j.Status.Succeeded, "Failures", j.Status.Failed)
		if job.IsCompleted(*j) {
			if !job.IsFailed(*j) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    tufConstants.RepositoryCondition,
					Status:  metav1.ConditionTrue,
					Reason:  constants.Ready,
					Message: "tuf-repository-init job passed",
				})
				return i.StatusUpdate(ctx, instance)
			} else {
				err = fmt.Errorf("tuf-repository-init job failed")
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    tufConstants.RepositoryCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: err.Error(),
				})
				return i.Error(ctx, reconcile.TerminalError(err), instance)
			}
		} else {
			// job not completed yet
			return i.Requeue()
		}
	} else if client.IgnoreNotFound(err) != nil {
		return i.Error(ctx, err, instance)

	}

	l := labels.For(tufConstants.ComponentName, tufConstants.InitJobName, instance.Name)
	pvc, err := kubernetes.GetPVC(ctx, i.Client, instance.Namespace, instance.Status.PvcName)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve PVC: %w", err), instance)
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tufConstants.InitJobName,
				Namespace: instance.Namespace,
			},
		},
		utils.EnsureTufInitJob(instance, tufConstants.RBACName, l),
		ensure.ControllerReference[*v2.Job](pvc, i.Client),
		ensure.Labels[*v2.Job](maps.Keys(l), l),
		func(object *v2.Job) error {
			ensure.SetProxyEnvs(object.Spec.Template.Spec.Containers)
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF init job: %w", err), instance)
	}

	i.Recorder.Event(instance, v1.EventTypeNormal, "JobCreated", "Tuf init-repository job created.")
	return i.Requeue()
}
