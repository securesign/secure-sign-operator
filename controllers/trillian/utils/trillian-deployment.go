package trillianUtils

import (
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTrillDeployment(namespace string, image string, dpName string, dbsecret string, labels map[string]string, serviceAccountName string) *apps.Deployment {
	replicas := int32(1)
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					ServiceAccountName: serviceAccountName,
					InitContainers: []core.Container{
						{
							Name:  "wait-for-trillian-db",
							Image: constants.TrillianNetcatImage,
							Env: []core.EnvVar{
								{
									Name: "MYSQL_HOSTNAME",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-host",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name: "MYSQL_PORT",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-port",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
							},
							Command: []string{
								"sh",
								"-c",
								"until nc -z -v -w30 $MYSQL_HOSTNAME $MYSQL_PORT; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
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
									Name: "MYSQL_USER",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-user",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
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
									Name: "MYSQL_HOSTNAME",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-host",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name: "MYSQL_PORT",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-port",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name: "MYSQL_DATABASE",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-database",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
