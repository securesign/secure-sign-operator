package trillianUtils

import (
	"errors"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTrillDb(instance *v1alpha1.Trillian, dpName string, sa string, openshift bool, labels map[string]string) (*apps.Deployment, error) {
	if instance.Status.Db.DatabaseSecretRef == nil {
		return nil, errors.New("reference to database secret is not set")
	}
	if instance.Status.Db.Pvc.Name == "" {
		return nil, errors.New("reference to database pvc is not set")
	}
	replicas := int32(1)
	var secCont *core.PodSecurityContext
	if !openshift {
		uid := int64(1001)
		fsid := int64(1001)
		secCont = &core.PodSecurityContext{
			RunAsUser: &uid,
			FSGroup:   &fsid,
		}
	}
	return &apps.Deployment{
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
					SecurityContext:    secCont,
					Volumes: []core.Volume{
						{
							Name: "storage",
							VolumeSource: core.VolumeSource{
								PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.Status.Db.Pvc.Name,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: constants.TrillianDbImage,
							ReadinessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									Exec: &core.ExecAction{
										Command: []string{
											"bash",
											"-c",
											"mariadb -u ${MYSQL_USER} -p${MYSQL_PASSWORD} -e \"SELECT 1;\"",
										},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									Exec: &core.ExecAction{
										Command: []string{
											"bash",
											"-c",
											"mariadb-admin -u ${MYSQL_USER} -p${MYSQL_PASSWORD} ping",
										},
									},
								},
								InitialDelaySeconds: 30,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 3306,
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
									Name: "MYSQL_ROOT_PASSWORD",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-root-password",
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
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "storage",
									MountPath: "/var/lib/mysql",
								},
							},
						},
					},
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: "Recreate",
			},
		},
	}, nil
}
