package kubernetes

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateJob(namespace string, name string, labels map[string]string, image string, serviceAccountName string, parallelism int32, completions int32, activeDeadlineSeconds int64, backoffLimit int32, command []string, env []corev1.EnvVar) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
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
