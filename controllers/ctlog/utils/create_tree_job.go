package utils

import (
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CTJob(namespace string, jobName string) *batch.Job {

	imageName := "registry.redhat.io/rhtas-tech-preview/createtree-rhel9@sha256:8a80def74e850f2b4c73690f86669a1fe52c1043c175610750abb4644e63d4ab"

	// Define a new Namespace object
	return &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "server",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      "ctlog",
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "server",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      "ctlog",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: "sigstore-sa",
					RestartPolicy:      core.RestartPolicyNever,
					Containers: []core.Container{
						{
							Name:  "trusted-artifact-signer-rekor-createtree",
							Image: imageName,
							Args: []string{
								"--namespace=$(NAMESPACE)",
								"--configmap=ctlog-config",
								"--display_name=ctlog-tree",
								"--admin_server=trillian-logserver." + namespace + ":8091",
							},
							Env: []core.EnvVar{
								{
									Name: "NAMESPACE",
									ValueFrom: &core.EnvVarSource{
										FieldRef: &core.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Resources: core.ResourceRequirements{},
						},
					},
				},
			},
		},
	}
}
