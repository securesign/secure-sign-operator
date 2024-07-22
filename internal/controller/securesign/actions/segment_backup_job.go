package actions

import (
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"

	"github.com/securesign/operator/internal/controller/annotations"

	"context"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"

	"github.com/operator-framework/operator-lib/proxy"
)

func NewSegmentBackupJobAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &segmentBackupJob{}
}

type segmentBackupJob struct {
	action.BaseAction
}

func (i segmentBackupJob) Name() string {
	return "segment-backup-installation"
}

func (i segmentBackupJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	if c := meta.FindStatusCondition(instance.Status.Conditions, SegmentBackupJobName); c != nil {
		return c.Reason != constants.Ready
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
	)

	if c := meta.FindStatusCondition(instance.Status.Conditions, SegmentBackupJobName); c == nil {
		instance.SetCondition(v1.Condition{
			Type:    SegmentBackupJobName,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Creating Segment Backup Job",
		})
	}

	labels := constants.LabelsFor(SegmentBackupJobName, SegmentBackupJobName, instance.Name)
	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)
	command := []string{"python3", "/opt/app-root/src/src/script.py"}
	env := []corev1.EnvVar{
		{
			Name:  "RUN_TYPE",
			Value: "installation",
		},
		{
			Name:  "REQUESTS_CA_BUNDLE",
			Value: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		},
	}

	// Adding proxy variables to operand
	env = append(env, proxy.ReadProxyVarsFromEnv()...)

	// Logic to delete old SBJ to avoid SECURESIGN-1207, can be removed after next release
	if sbj, err := kubernetes.GetJob(ctx, i.Client, instance.Namespace, SegmentBackupJobName); sbj != nil {
		if err = i.Client.Delete(ctx, sbj); err != nil {
			i.Logger.Error(err, "problem with removing SBJ resources", "namespace", instance.Namespace, "name", SegmentBackupJobName)
		}
	} else if client.IgnoreNotFound(err) != nil {
		i.Logger.Error(err, "unable to retrieve SBJ resource", "namespace", instance.Namespace, "name", SegmentBackupJobName)
	}

	job := kubernetes.CreateJob(instance.Namespace, SegmentBackupJobName, labels, constants.SegmentBackupImage, SegmentRBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Job: %w", err))
	}

	_, err = i.Ensure(ctx, job)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the job: %w", err))
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    SegmentBackupJobName,
		Status:  metav1.ConditionTrue,
		Reason:  constants.Ready,
		Message: "Segment Backup Job Created",
	})

	return i.Continue()
}
