package backfillredis

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewBackfillRedisCronJobAction() action.Action[*rhtasv1.Rekor] {
	return &backfillRedisCronJob{}
}

type backfillRedisCronJob struct {
	action.BaseAction
}

func (i backfillRedisCronJob) Name() string {
	return "backfill-redis"
}

func (i backfillRedisCronJob) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i backfillRedisCronJob) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	if _, err := cron.ParseStandard(instance.Spec.BackFillRedis.Schedule); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create backfill redis cron job: %w", err)), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	labels := labels.For(actions.BackfillRedisCronJobName, actions.BackfillRedisCronJobName, instance.Name)

	job, err := traits.Apply(
		batchv1ac.CronJob(actions.BackfillRedisCronJobName, instance.Namespace),
		traits.ControllerReference[*batchv1ac.CronJobApplyConfiguration](instance, i.Client),
		traits.Labels[*batchv1ac.CronJobApplyConfiguration](labels),
		i.ensureBackfillCronJob(instance),
		i.ensureSearchIndex(instance),
		traits.WithCronJobPodSpecTraits(
			traits.SecurityContexts(actions.BackfillRedisCronJobName),
			traits.GODEBUG(instance.GetAnnotations(), actions.BackfillRedisCronJobName),
			traits.Auth(instance.Spec.Auth, actions.BackfillRedisCronJobName),
			traits.TrustedCA(instance.GetTrustedCA(), actions.BackfillRedisCronJobName),
			traits.ServiceAccount(actions.RBACName),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build backfill redis CronJob: %w", err), instance)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, job,
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.BackfillRedisCronJobName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s CronJob: %w", actions.BackfillRedisCronJobName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Backfill redis job created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i backfillRedisCronJob) ensureBackfillCronJob(instance *rhtasv1.Rekor) traits.Trait[*batchv1ac.CronJobApplyConfiguration] {
	return func(job *batchv1ac.CronJobApplyConfiguration) error {
		container := corev1ac.Container().
			WithName(actions.BackfillRedisCronJobName).
			WithImage(images.Registry.Get(images.BackfillRedis)).
			WithCommand("/bin/sh", "-c").
			WithArgs(
				fmt.Sprintf(
					`endIndex=$(curl -sS http://%s/api/v1/log | sed -E 's/.*"treeSize":([0-9]+).*/\1/'); endIndex=$((endIndex-1)); if [ $endIndex -lt 0 ]; then echo "info: no rekor entries found"; exit 0; fi; backfill-redis --rekor-address=http://%s --start=0 --end=$endIndex`,
					actions.ServerComponentName,
					actions.ServerComponentName,
				),
			)

		job.WithSpec(batchv1ac.CronJobSpec().
			WithSchedule(instance.Spec.BackFillRedis.Schedule).
			WithJobTemplate(batchv1ac.JobTemplateSpec().
				WithSpec(batchv1ac.JobSpec().
					WithTemplate(corev1ac.PodTemplateSpec().WithLabels(job.Labels).
						WithSpec(corev1ac.PodSpec().
							WithContainers(container).
							WithRestartPolicy(corev1.RestartPolicyOnFailure))))))
		return nil
	}
}

func (i backfillRedisCronJob) ensureSearchIndex(instance *rhtasv1.Rekor) traits.Trait[*batchv1ac.CronJobApplyConfiguration] {
	return func(job *batchv1ac.CronJobApplyConfiguration) error {
		options, err := searchIndex.Resolve(instance)
		if err != nil {
			return err
		}
		container := kubernetes.FindContainerByName(job.Spec.JobTemplate.Spec.Template.Spec, actions.BackfillRedisCronJobName)
		if ref := options.PasswordRef; ref != nil {
			container.WithEnv(corev1ac.EnvVar().
				WithName(searchIndex.RedisPasswordEnv).
				WithValueFrom(corev1ac.EnvVarSource().
					WithSecretKeyRef(corev1ac.SecretKeySelector().WithName(ref.Name).WithKey(ref.Key))))
		}
		// the first argumet is the script - we must append Args[0]
		if options.Redis != nil {
			container.Args[0] += ensureRedisParams(options.Redis)
		} else {
			container.Args[0] += ensureMysqlParams(options.MySQLDSN)
		}
		return nil
	}
}

func ensureRedisParams(options *redis.RedisOptions) string {
	params := fmt.Sprintf(" --redis-hostname=\"%s\"", envAsShellParams(options.Host))

	if options.Port != "" {
		params += fmt.Sprintf(" --redis-port=\"%s\"", envAsShellParams(options.Port))
	}

	if options.Password != "" {
		params += fmt.Sprintf(" --redis-password=\"%s\"", envAsShellParams(options.Password))
	}
	if options.TlsEnabled {
		params += " --redis-enable-tls=\"true\""
	}
	return params
}

func ensureMysqlParams(url string) string {
	return fmt.Sprintf(" --mysql-dsn=\"%s\"", envAsShellParams(url))
}
