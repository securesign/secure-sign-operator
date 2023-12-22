package utils

import (
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	netcat = "registry.redhat.io/rhtas-tech-preview/trillian-netcat-rhel9@sha256:b9fa895af8967cceb7a05ed7c9f2b80df047682ed11c87249ca2edba86492f6e"
)

func CreateTrillDeployment(namespace string, image string, dpName string, dbsecret string) *apps.Deployment {
	replicas := int32(1)
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": dpName,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      "trillian",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": dpName,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
					"app.kubernetes.io/name":      "trillian",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": dpName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      "trillian",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: "sigstore-sa",
					InitContainers: []core.Container{
						{
							Name:  "wait-for-trillian-db",
							Image: netcat,
							Command: []string{
								"sh",
								"-c",
								"until nc -z -v -w30 trillian-mysql 3306; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
							},
						},
					},
					Containers: []core.Container{
						{
							Args: []string{
								"--storage_system=mysql",
								"--quota_system=mysql",
								"--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/$(MYSQL_DATABASE)",
								"--rpc_endpoint=0.0.0.0:8091",
								"--http_endpoint=0.0.0.0:8090",
								"--alsologtostderr",
							},
							Name:  dpName,
							Image: image,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 8091,
								},
							},
							// Env variables from secret trillian-mysql
							Env: []core.EnvVar{
								{
									Name:  "MYSQL_USER",
									Value: "mysql",
								},
								{
									Name: "MYSQL_PASSWORD",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-password",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name:  "MYSQL_HOSTNAME",
									Value: "trillian-mysql",
								},
								{
									Name:  "MYSQL_PORT",
									Value: "3306",
								},
								{
									Name:  "MYSQL_DATABASE",
									Value: "trillian",
								},
							},
						},
					},
				},
			},
		},
	}
}
