package kubernetes

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	jobNameTemplate = "%s-"
)

func CreateJob(namespace string, name string, labels map[string]string, image string, serviceAccountName string, parallelism int32, completions int32, activeDeadlineSeconds int64, backoffLimit int32, command []string, env []corev1.EnvVar) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(jobNameTemplate, name),
			Namespace:    namespace,
			Labels:       labels,
		},
		Spec: batchv1.JobSpec{
			Parallelism:           &parallelism,
			Completions:           &completions,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			BackoffLimit:          &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					RestartPolicy:      "OnFailure",
					Containers: []corev1.Container{
						{
							Name:    name,
							Image:   image,
							Command: command,
							Env:     env,
						},
					},
				},
			},
		},
	}
}

func GetJob(ctx context.Context, c client.Client, namespace, jobName string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: jobName}, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}
