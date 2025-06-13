package backfillredis

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/robfig/cron/v3"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
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
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
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
		func(object *batchv1.CronJob) error {
			return ensure.Auth(actions.BackfillRedisCronJobName, instance.Spec.Auth)(&object.Spec.JobTemplate.Spec.Template.Spec)
		},
		ensure.ControllerReference[*batchv1.CronJob](instance, i.Client),
		ensure.Labels[*batchv1.CronJob](slices.Collect(maps.Keys(labels)), labels),
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
			fmt.Sprintf(`endIndex=$(curl -sS http://%s/api/v1/log | sed -E 's/.*"treeSize":([0-9]+).*/\1/'); endIndex=$((endIndex-1)); if [ $endIndex -lt 0 ]; then echo "info: no rekor entries found"; exit 0; fi; backfill-redis --rekor-address=http://%s --start=0 --end=$endIndex`, actions.ServerComponentName, actions.ServerComponentName),
		}
		searchParams, err := i.searchIndexParams(*instance)
		if err != nil {
			return err
		}
		container.Args[0] = fmt.Sprintf(" %s %s", container.Args[0], strings.Join(searchParams, " "))
		return nil
	}
}

func (i backfillRedisCronJob) searchIndexParams(instance rhtasv1alpha1.Rekor) ([]string, error) {
	args := make([]string, 0)
	switch instance.Spec.SearchIndex.Provider {
	case "redis":
		options, err := redis.Parse(instance.Spec.SearchIndex.Url)
		if err != nil {
			return nil, fmt.Errorf("can't parse redis searchIndex url: %w", err)
		}
		args = append(args, fmt.Sprintf("--redis-hostname=\"%s\"", options.Host))

		if options.Port != "" {
			args = append(args, fmt.Sprintf("--redis-port=\"%s\"", options.Port))
		}

		if options.Password != "" {
			args = append(args, fmt.Sprintf("--redis-password=\"%s\"", options.Password))
		}
		return args, nil
	case "mysql":
		return append(args, fmt.Sprintf("--mysql-dsn=\"%s\"", instance.Spec.SearchIndex.Url)), nil
	default:
		return nil, fmt.Errorf("unsupported search_index provider %s", instance.Spec.SearchIndex.Provider)
	}
}
