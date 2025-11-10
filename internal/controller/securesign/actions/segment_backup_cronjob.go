package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/robfig/cron/v3"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
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
	if !kubernetes.IsOpenShift() {
		return false
	}

	c := meta.FindStatusCondition(instance.Status.Conditions, MetricsCondition)
	if c == nil || c.Reason == constants.Ready {
		return false
	}
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
		err    error
		result controllerutil.OperationResult
	)

	if _, err := cron.ParseStandard(AnalyiticsCronSchedule); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create segment backuup cron job due to errors with parsing the cron schedule: %w", err), instance)
	}

	labels := labels.For(SegmentBackupCronJobName, SegmentBackupCronJobName, instance.Name)

	segmentBackupCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentBackupCronJobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		segmentBackupCronJob,
		i.ensureSegmentBackupCronJob(),
		ensure.ControllerReference[*batchv1.CronJob](instance, i.Client),
		ensure.Labels[*batchv1.CronJob](slices.Collect(maps.Keys(labels)), labels),
		func(object *batchv1.CronJob) error {
			ensure.SetProxyEnvs(object.Spec.JobTemplate.Spec.Template.Spec.Containers)
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create segment backup cron job: %w", err), instance,
			metav1.Condition{
				Type:    MetricsCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionTrue,
			Reason:  constants.Ready,
			Message: "Segment backup Cron Job created",
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}

func (i segmentBackupCronJob) ensureSegmentBackupCronJob() func(job *batchv1.CronJob) error {
	return func(job *batchv1.CronJob) error {
		{
			spec := &job.Spec
			spec.Schedule = AnalyiticsCronSchedule

			templateSpec := &spec.JobTemplate.Spec.Template.Spec
			templateSpec.ServiceAccountName = SegmentRBACName
			templateSpec.RestartPolicy = "OnFailure"

			container := kubernetes.FindContainerByNameOrCreate(templateSpec, SegmentBackupCronJobName)
			container.Image = images.Registry.Get(images.SegmentBackup)
			container.Command = []string{"python3", "/opt/app-root/src/src/script.py"}

			runTypeEnv := kubernetes.FindEnvByNameOrCreate(container, "RUN_TYPE")
			runTypeEnv.Value = "nightly"

			caBundleEnv := kubernetes.FindEnvByNameOrCreate(container, "REQUESTS_CA_BUNDLE")
			caBundleEnv.Value = "/etc/pki/tls/certs/ca-bundle.crt" // Certificate used to verify requests externally i.e communication with segment

			internalCaBundleEnv := kubernetes.FindEnvByNameOrCreate(container, "REQUESTS_CA_BUNDLE_INTERNAL")
			internalCaBundleEnv.Value = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt" // Certificate used to verify requests internally i.e queries to thanos

		}
		return nil
	}
}
