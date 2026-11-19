package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	jobUtils "github.com/securesign/operator/internal/utils/kubernetes/job"
	v2 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apilabels "k8s.io/apimachinery/pkg/labels"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewInitJobAction() action.Action[*rhtasv1.Tuf] {
	return &initJobAction{}
}

type initJobAction struct {
	action.BaseAction
}

func (i initJobAction) Name() string {
	return "tuf-init job"
}

func (i initJobAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	return !meta.IsStatusConditionTrue(instance.GetConditions(), tufConstants.RepositoryCondition) &&
		state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i initJobAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	jobLabels := labels.ForResource(tufConstants.ComponentName, tufConstants.InitJobName, instance.Name, instance.Status.PvcName)
	initJobList := &v2.JobList{}
	selector := apilabels.SelectorFromSet(jobLabels)
	if err := kubernetes.FindByLabelSelector(ctx, i.Client, initJobList, instance.Namespace, selector.String()); err != nil {
		return i.Error(ctx, err, instance)
	}

	switch {
	case len(initJobList.Items) > 1:
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("multiple %s jobs present", tufConstants.InitJobName)), instance)
	case len(initJobList.Items) == 1:
		return i.jobPresent(ctx, &initJobList.Items[0], instance)
	default:
		return i.ensureInitJob(ctx, jobLabels, instance)
	}
}

func (i initJobAction) jobPresent(ctx context.Context, job *v2.Job, instance *rhtasv1.Tuf) *action.Result {
	i.Logger.Info("Tuf tuf-repository-init is present.", "Succeeded", job.Status.Succeeded, "Failures", job.Status.Failed)
	if jobUtils.IsCompleted(*job) {
		if !jobUtils.IsFailed(*job) {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    tufConstants.RepositoryCondition,
				Status:  metav1.ConditionTrue,
				Reason:  state.Ready.String(),
				Message: "tuf-repository-init job passed",
			})
			if job.Annotations[tufConstants.RepositoryVersionAnnotation] == tufConstants.TufVersionV1 {
				// annotate self to signal that the tuf repository is already v1
				if err := markRepositoryV1(ctx, i.Client, instance); err != nil {
					return i.Error(ctx, err, instance)
				}
			}
			return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
		} else {
			err := fmt.Errorf("tuf-repository-init job failed")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    tufConstants.RepositoryCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
	} else {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    tufConstants.RepositoryCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Initialize.String(),
			Message: "waiting for init-repository job to complete",
		})
		if _, err := i.PersistStatus(ctx, instance); err != nil {
			return i.Error(ctx, err, instance)
		}
		return i.RequeueAfter(5 * time.Second)
	}
}

func markRepositoryV1(ctx context.Context, cli client.Client, instance *rhtasv1.Tuf) error {
	if instance.Annotations[tufConstants.RepositoryVersionAnnotation] == tufConstants.TufVersionV1 {
		return nil
	}

	updated := instance.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations[tufConstants.RepositoryVersionAnnotation] = tufConstants.TufVersionV1
	patch := client.MergeFromWithOptions(instance, client.MergeFromWithOptimisticLock{})
	if err := cli.Patch(ctx, updated, patch); err != nil {
		return fmt.Errorf("could not mark TUF repository as v1: %w", err)
	}
	instance.ObjectMeta = updated.ObjectMeta
	return nil
}

func (i initJobAction) ensureInitJob(ctx context.Context, labels map[string]string, instance *rhtasv1.Tuf) *action.Result {
	if instance.Spec.RootKeySecretRef == nil {
		err := fmt.Errorf("rootKeySecretRef is not set")
		return i.Error(ctx, reconcile.TerminalError(err), instance, metav1.Condition{
			Type:    tufConstants.RepositoryCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}
	jobConfig, err := traits.Apply(
		batchv1ac.Job("", instance.Namespace).WithGenerateName(tufConstants.InitJobName+"-"),
		traits.Annotations[*batchv1ac.JobApplyConfiguration](map[string]string{tufConstants.RepositoryVersionAnnotation: tufConstants.TufVersionV1}),
		traits.ControllerReference[*batchv1ac.JobApplyConfiguration](instance, i.Client),
		traits.Labels[*batchv1ac.JobApplyConfiguration](labels),
		utils.EnsureTufInitJob(ctx, i.Client, instance),
		traits.WithJobPodSpecTraits(
			traits.Proxy([]string{utils.TufInitContainerName}),
			traits.GODEBUG(instance.GetAnnotations(), utils.TufInitContainerName),
			traits.SecurityContexts(utils.TufInitContainerName),
			traits.TrustedCA(instance.GetTrustedCA(), utils.TufInitContainerName),
			traits.ServiceAccount(tufConstants.RBACInitJobName),
		),
	)
	if err != nil {
		if errors.Is(err, utils.ErrorResolveServiceUrl) {
			return i.Error(ctx, err, instance,
				metav1.Condition{
					Type:               tufConstants.RepositoryCondition,
					Status:             metav1.ConditionFalse,
					Reason:             state.Pending.String(),
					Message:            err.Error(),
					ObservedGeneration: instance.GetGeneration(),
				},
			)
		}
		return i.Error(ctx, fmt.Errorf("could not create TUF init job: %w", err), instance)
	}
	if _, err := kubernetes.Create(ctx, i.Client, jobConfig); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF init job: %w", err), instance)
	}

	msg := "Tuf init-repository job created."
	i.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "JobCreated", "Created", msg)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               tufConstants.RepositoryCondition,
		Status:             metav1.ConditionFalse,
		Reason:             state.Initialize.String(),
		Message:            msg,
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
