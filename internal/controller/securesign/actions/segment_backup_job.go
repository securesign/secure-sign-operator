package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	batchv1 "k8s.io/api/batch/v1"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSegmentBackupJobAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &segmentBackupJob{}
}

type segmentBackupJob struct {
	action.BaseAction
}

func (i segmentBackupJob) Name() string {
	return SegmentBackupJobName
}

func (i segmentBackupJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
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

func (i segmentBackupJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err error
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: SegmentBackupJobName + "-",
				Namespace:    instance.Namespace,
			},
		}
	)

	l := labels.For(SegmentBackupJobName, SegmentBackupJobName, instance.Name)
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		job,
		i.ensureSegmentBackupJob(),
		ensure.ControllerReference[*batchv1.Job](instance, i.Client),
		ensure.Labels[*batchv1.Job](slices.Collect(maps.Keys(l)), l),
		func(object *batchv1.Job) error {
			ensure.SetProxyEnvs(object.Spec.Template.Spec.Containers)
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create segment backup job: %w", err), instance,
			metav1.Condition{
				Type:    MetricsCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Creating,
				Message: err.Error(),
			})
	}

	return i.Continue()
}

func (i segmentBackupJob) ensureSegmentBackupJob() func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		spec := &job.Spec
		spec.Parallelism = utils.Pointer[int32](1)
		spec.Completions = utils.Pointer[int32](1)
		spec.ActiveDeadlineSeconds = utils.Pointer[int64](600)
		spec.BackoffLimit = utils.Pointer[int32](5)

		templateSpec := &spec.Template.Spec
		templateSpec.ServiceAccountName = SegmentRBACName
		templateSpec.RestartPolicy = "OnFailure"

		container := kubernetes.FindContainerByNameOrCreate(templateSpec, SegmentBackupJobName)
		container.Image = images.Registry.Get(images.SegmentBackup)
		container.Command = []string{"python3", "/opt/app-root/src/src/script.py"}

		runTypeEnv := kubernetes.FindEnvByNameOrCreate(container, "RUN_TYPE")
		runTypeEnv.Value = "installation"

		caBundleEnv := kubernetes.FindEnvByNameOrCreate(container, "REQUESTS_CA_BUNDLE")
		caBundleEnv.Value = "/etc/pki/tls/certs/ca-bundle.crt" // Certificate used to verify requests externally i.e communication with segment

		internalCaBundleEnv := kubernetes.FindEnvByNameOrCreate(container, "REQUESTS_CA_BUNDLE_INTERNAL")
		internalCaBundleEnv.Value = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt" // Certificate used to verify requests internally i.e queries to thanos
		return nil
	}
}
