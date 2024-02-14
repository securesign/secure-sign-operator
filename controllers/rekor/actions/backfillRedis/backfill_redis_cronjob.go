package backfillredis

import (
	"fmt"

	"github.com/robfig/cron/v3"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"context"

	"github.com/securesign/operator/controllers/common/action"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewBackfillRedisCronJobAction() action.Action[rhtasv1alpha1.Rekor] {
	return &backfillRedisCronJob{}
}

type backfillRedisCronJob struct {
	action.BaseAction
}

func (i backfillRedisCronJob) Name() string {
	return "backfill-redis"
}

func (i backfillRedisCronJob) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return (instance.Status.Phase == rhtasv1alpha1.PhaseCreating || instance.Status.Phase == rhtasv1alpha1.PhaseReady) && instance.Spec.BackFillRedis.Enabled
}

func (i backfillRedisCronJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)

	if _, err := cron.ParseStandard(instance.Spec.BackFillRedis.Schedule); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.Failed(fmt.Errorf("could not create backfill redis cron job: %w", err))
	}

	labels := constants.LabelsFor(actions.BackfillRedisCronJobName, actions.BackfillRedisCronJobName, instance.Name)
	backfillRedisCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      actions.BackfillRedisCronJobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: instance.Spec.BackFillRedis.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: actions.RBACName,
							RestartPolicy:      "OnFailure",
							Containers: []corev1.Container{
								{
									Name:    actions.BackfillRedisCronJobName,
									Image:   constants.BackfillRedisImage,
									Command: []string{"/bin/sh", "-c"},
									Args: []string{
										fmt.Sprintf(`endIndex=$(curl -sS http://%s/api/v1/log | sed -E 's/.*"treeSize":([0-9]+).*/\1/'); endIndex=$((endIndex-1)); if [ $endIndex -lt 0 ]; then echo "info: no rekor entries found"; exit 0; fi; backfill-redis --hostname=rekor-redis --port=6379 --rekor-address=http://%s --start=0 --end=$endIndex`, actions.ServerComponentName, actions.ServerComponentName),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err = controllerutil.SetControllerReference(instance, backfillRedisCronJob, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for backfill redis cron job: %w", err))
	}

	if updated, err = i.Ensure(ctx, backfillRedisCronJob); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create backfill redis cron job: %w", err), instance)
	}

	if updated {
		return i.Return()
	} else {
		return i.Continue()
	}
}
