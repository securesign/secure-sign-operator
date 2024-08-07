package trillianUtils

import (
	"errors"
	"strconv"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTrillDeployment(instance *v1alpha1.Trillian, image string, dpName string, sa string, labels map[string]string) (*apps.Deployment, error) {
	if instance.Status.Db.DatabaseSecretRef == nil {
		return nil, errors.New("reference to database secret is not set")
	}
	replicas := int32(1)
	containerPorts := []core.ContainerPort{
		{
			Protocol:      core.ProtocolTCP,
			ContainerPort: 8091,
		},
	}

	if instance.Spec.Monitoring.Enabled {
		containerPorts = append(containerPorts, core.ContainerPort{
			Protocol:      core.ProtocolTCP,
			ContainerPort: 8090,
		})
	}

	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: instance.Namespace,
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
					ServiceAccountName: sa,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
								"--rpc_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.ServerPort)),
								"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.MetricsPort)),
								"--alsologtostderr",
							},
							Name:  dpName,
							Image: image,
							Ports: containerPorts,
							// Env variables from secret trillian-mysql
							Env: []core.EnvVar{
								{
									Name: "MYSQL_USER",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-user",
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
												Name: instance.Status.Db.DatabaseSecretRef.Name,
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
	utils.SetProxyEnvs(dep)
	return dep, nil
}
