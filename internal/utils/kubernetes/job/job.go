package job

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetJob(ctx context.Context, c client.Client, namespace, jobName string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: jobName}, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func IsCompleted(job batchv1.Job) bool {
	completed := getJobCondition(job.Status.Conditions, batchv1.JobComplete)
	if completed != nil {
		return completed.Status == corev1.ConditionTrue
	}
	return false
}

func IsFailed(job batchv1.Job) bool {
	fail := getJobCondition(job.Status.Conditions, batchv1.JobFailed)
	if fail != nil {
		return fail.Status == corev1.ConditionTrue
	}
	return false
}

func getJobCondition(conditions []batchv1.JobCondition, condType batchv1.JobConditionType) *batchv1.JobCondition {
	for _, c := range conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
