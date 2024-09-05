package utils

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateTufDeployment(instance *v1alpha1.Tuf, dpName string, sa string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)
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
					Volumes: []core.Volume{
						{
							Name: "repository",
							VolumeSource: core.VolumeSource{
								PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.Status.PvcName,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  "tuf-server",
							Image: constants.HttpServerImage,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "repository",
									MountPath: "/var/www/html",
									ReadOnly:  true,
								},
							},
							LivenessProbe: &core.Probe{
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								FailureThreshold:    3,
								SuccessThreshold:    1,
								ProbeHandler: core.ProbeHandler{
									// server is running returning any status code (including 403 - noindex.html)
									Exec: &core.ExecAction{Command: []string{"curl", "localhost:8080"}},
								},
							},
							ReadinessProbe: &core.Probe{
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								FailureThreshold:    10,
								SuccessThreshold:    1,
								ProbeHandler: core.ProbeHandler{
									HTTPGet: &core.HTTPGetAction{
										Port: intstr.FromInt32(8080),
										Path: "/root.json",
									},
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
	}
	utils.SetProxyEnvs(dep)
	return dep
}
