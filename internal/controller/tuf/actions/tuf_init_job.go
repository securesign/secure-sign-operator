package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	return c.Reason == constants.Creating && !meta.IsStatusConditionTrue(instance.GetConditions(), RepositoryCondition)
}

func (i initJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	if instance.Spec.Pvc.Name != "" {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    RepositoryCondition,
			Status:  metav1.ConditionTrue,
			Reason:  constants.Ready,
			Message: "Using self-managed tuf repository.",
		})
		return i.StatusUpdate(ctx, instance)
	}

	if j, err := job.GetJob(ctx, i.Client, instance.Namespace, InitJobName); j != nil {
		i.Logger.Info("Tuf tuf-repository-init is already present.", "Succeeded", j.Status.Succeeded, "Failures", j.Status.Failed)
		if job.IsCompleted(*j) {
			if !job.IsFailed(*j) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    RepositoryCondition,
					Status:  metav1.ConditionTrue,
					Reason:  constants.Ready,
					Message: "tuf-repository-init job passed",
				})
				return i.StatusUpdate(ctx, instance)
			} else {
				err = fmt.Errorf("tuf-repository-init job failed")
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    RepositoryCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: err.Error(),
				})
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    constants.Ready,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: err.Error(),
				})
				return i.FailedWithStatusUpdate(ctx, err, instance)
			}
		} else {
			// job not completed yet
			return i.Requeue()
		}
	} else if client.IgnoreNotFound(err) != nil {
		return i.Failed(err)

	}
	j := utils.CreateTufInitJob(instance, InitJobName, RBACName, constants.LabelsForComponent(ComponentName, instance.Name))
	if err := controllerutil.SetControllerReference(instance, j, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Job: %w", err))
	}
	if err := i.Client.Create(ctx, j); err != nil {
		return i.Failed(err)
	}
	i.Recorder.Event(instance, v1.EventTypeNormal, "JobCreated", "Tuf init-repository job created.")
	return i.Requeue()
}
