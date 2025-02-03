package backfillredis

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/images"

	"github.com/robfig/cron/v3"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"golang.org/x/exp/maps"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewBackfillRedisCronJobAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &backfillRedisCronJob{}
}

type backfillRedisCronJob struct {
	action.BaseAction
}

func (i backfillRedisCronJob) Name() string {
	return "backfill-redis"
}

func (i backfillRedisCronJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && utils.OptionalBool(instance.Spec.BackFillRedis.Enabled)
}

func (i backfillRedisCronJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	if _, err := cron.ParseStandard(instance.Spec.BackFillRedis.Schedule); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create backfill redis cron job: %w", err)), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	labels := labels.For(actions.BackfillRedisCronJobName, actions.BackfillRedisCronJobName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.BackfillRedisCronJobName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureBacfillCronJob(instance),
		ensure.ControllerReference[*batchv1.CronJob](instance, i.Client),
		ensure.Labels[*batchv1.CronJob](maps.Keys(labels), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s CronJob: %w", actions.BackfillRedisCronJobName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Backfill redis job created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i backfillRedisCronJob) ensureBacfillCronJob(instance *rhtasv1alpha1.Rekor) func(*batchv1.CronJob) error {
	return func(job *batchv1.CronJob) error {
		job.Spec.Schedule = instance.Spec.BackFillRedis.Schedule
		templateSpec := &job.Spec.JobTemplate.Spec.Template.Spec
		templateSpec.ServiceAccountName = actions.RBACName
		templateSpec.RestartPolicy = "OnFailure"

		container := kubernetes.FindContainerByNameOrCreate(templateSpec, actions.BackfillRedisCronJobName)

		container.Image = images.Registry.Get(images.BackfillRedis)
		container.Command = []string{"/bin/sh", "-c"}
		container.Args = []string{
			fmt.Sprintf(`endIndex=$(curl -sS http://%s/api/v1/log | sed -E 's/.*"treeSize":([0-9]+).*/\1/'); endIndex=$((endIndex-1)); if [ $endIndex -lt 0 ]; then echo "info: no rekor entries found"; exit 0; fi; backfill-redis --redis-hostname=rekor-redis --redis-port=6379 --rekor-address=http://%s --start=0 --end=$endIndex`, actions.ServerComponentName, actions.ServerComponentName),
		}
		return nil
	}
}
