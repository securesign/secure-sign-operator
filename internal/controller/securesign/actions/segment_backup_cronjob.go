package actions

import (
	"fmt"
	"strconv"

	"github.com/securesign/operator/internal/controller/annotations"

	"github.com/robfig/cron/v3"
	"github.com/securesign/operator/internal/controller/constants"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
)

func NewSegmentBackupCronJobAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &segmentBackupCronJob{}
}

type segmentBackupCronJob struct {
	action.BaseAction
}

func (i segmentBackupCronJob) Name() string {
	return "segment-backup-nightly-metrics"
}
func (i segmentBackupCronJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	val, found := instance.Annotations[annotations.Metrics]
	if !found {
		return true
	}
	if boolVal, err := strconv.ParseBool(val); err == nil {
		return boolVal
	}
	return true
}

func (i segmentBackupCronJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)

	if _, err := cron.ParseStandard(AnalyiticsCronSchedule); err != nil {
		return i.Failed(fmt.Errorf("could not create segment backuup cron job due to errors with parsing the cron schedule: %w", err))
	}

	labels := constants.LabelsFor(SegmentBackupCronJobName, SegmentBackupCronJobName, instance.Name)

	segmentBackupCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentBackupCronJobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: AnalyiticsCronSchedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: SegmentRBACName,
							RestartPolicy:      "OnFailure",
							Containers: []corev1.Container{
								{
									Name:    SegmentBackupCronJobName,
									Image:   constants.SegmentBackupImage,
									Command: []string{"python3", "/opt/app-root/src/src/script.py"},
									Env: []corev1.EnvVar{
										{
											Name:  "RUN_TYPE",
											Value: "nightly",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err = controllerutil.SetControllerReference(instance, segmentBackupCronJob, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for segment backup cron job: %w", err))
	}

	if updated, err = i.Ensure(ctx, segmentBackupCronJob); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create segment backup cron job: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Segment backup Cron Job created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
