package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	return true
}

func (i segmentBackupCronJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	segmentBackupCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentBackupCronJobName,
			Namespace: instance.Namespace,
		},
	}

	err := i.Client.Delete(ctx, segmentBackupCronJob)
	if err != nil {
		if errors.IsNotFound(err) {
			return i.Continue()
		} else {
			return i.Error(ctx, fmt.Errorf("could not delete segment backup cron job: %w", err), instance, metav1.Condition{
				Type:    MetricsCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    MetricsCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "Removed",
		Message: "Segment backup Cron Job removed",
	})
	return i.StatusUpdate(ctx, instance)
}
