package actions

import (
	"fmt"
	"github.com/securesign/operator/internal/controller/annotations"
	"strconv"

	"context"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewSegmentBackupJobAction() action.Action[rhtasv1alpha1.Securesign] {
	return &segmentBackupJob{}
}

type segmentBackupJob struct {
	action.BaseAction
}

func (i segmentBackupJob) Name() string {
	return "segment-backup-installation"
}
func (i segmentBackupJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
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
	}

	job := kubernetes.CreateJob(instance.Namespace, SegmentBackupJobName, labels, constants.SegmentBackupImage, SegmentRBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for Job: %w", err))
	}
	_, err = i.Ensure(ctx, job)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the job: %w", err))
	}
	return i.Continue()
}
