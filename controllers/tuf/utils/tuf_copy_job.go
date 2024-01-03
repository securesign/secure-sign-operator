package utils

import (
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func InitTufCopyJob(namespace string, jobName string) *batch.Job {

	imageName := "registry.redhat.io/openshift4/ose-cli:latest"
	return &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": jobName,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      "tuf-secret-copy",
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": jobName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      "tuf-secret-copy",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: "sigstore-sa",
					RestartPolicy:      core.RestartPolicyOnFailure,
					Containers: []core.Container{
						{
							Name:    "copy-rekor-secret",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"curl rekor-server." + namespace + ".svc.cluster.local/api/v1/log/publicKey -o /tmp/key -v && kubectl create secret generic rekor-public-key --from-file=key=/tmp/key",
							},
						},
					}}}},
	}
}
