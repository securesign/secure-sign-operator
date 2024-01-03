package utils

import (
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateCTJob(namespace string, jobName string) *batch.Job {

	imageName := "registry.redhat.io/rhtas-tech-preview/createctconfig-rhel9@sha256:10155f8c2b73b12599124895b2db0c9e08b2c3953df7361574fd08467c42fd04"

	// Define a new Namespace object
	return &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "ctlog",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "mysql",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName:           "sigstore-sa",
					AutomountServiceAccountToken: &[]bool{true}[0],
					RestartPolicy:                core.RestartPolicyNever,
					InitContainers: []core.Container{
						{
							Name:  "wait-for-createtree-configmap",
							Image: "registry.redhat.io/openshift4/ose-cli:latest",
							Command: []string{
								"sh",
								"-c",
								"until curl --fail --header \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt --max-time 10 https://kubernetes.default.svc/api/v1/namespaces/" + namespace + "/configmaps/ctlog-config | grep '\"treeID\"':; do echo waiting for Configmap ctlog-config; sleep 5; done;",
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  "trusted-artifact-signer-ctlog-createctconfig",
							Image: imageName,
							Args: []string{
								"--configmap=ctlog-config",
								"--secret=ctlog-secret",
								"--pubkeysecret=ctlog-public-key",
								"--fulcio-url=http://fulcio-server." + namespace + ".svc",
								"--trillian-server=trillian-logserver." + namespace + ":8091",
								"--log-prefix=sigstorescaffolding",
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
						},
					},
				},
			},
		},
	}
}
