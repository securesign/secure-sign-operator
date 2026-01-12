package backfillredis

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/robfig/cron/v3"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
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
			ref := &object.Spec.JobTemplate.Spec.Template.Spec
			return ensure.Auth(actions.BackfillRedisCronJobName, instance.Spec.Auth)(ref)
		},
		func(object *batchv1.CronJob) error {
			return tlsensure.TrustedCA(instance.GetTrustedCA(), actions.BackfillRedisCronJobName)(&object.Spec.JobTemplate.Spec.Template)
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
		if err := searchIndex.EnsureSearchIndex(instance, ensureRedisParams(), ensureMysqlParams())(container); err != nil {
			return err
		}
		return nil
	}
}

func ensureRedisParams() func(*redis.RedisOptions, *v1.Container) {
	return func(options *redis.RedisOptions, container *v1.Container) {
		if len(container.Args) < 1 {
			container.Args = make([]string, 1)
		}
		container.Args[0] += fmt.Sprintf(" --redis-hostname=\"%s\"", envAsShellParams(options.Host))

		if options.Port != "" {
			container.Args[0] += fmt.Sprintf(" --redis-port=\"%s\"", envAsShellParams(options.Port))
		}

		if options.Password != "" {
			container.Args[0] += fmt.Sprintf(" --redis-password=\"%s\"", envAsShellParams(options.Password))
		}
		if options.TlsEnabled {
			container.Args[0] += " --redis-enable-tls=\"true\""
		}
	}
}

func ensureMysqlParams() func(string, *v1.Container) {
	return func(url string, container *v1.Container) {
		container.Args[0] += fmt.Sprintf(" --mysql-dsn=\"%s\"", envAsShellParams(url))
	}
}
