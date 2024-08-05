package utils

import (
	"fmt"
	"strconv"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateDeployment(instance *v1alpha1.CTlog, deploymentName string, sa string, labels map[string]string, serverPort, metricsPort int32, useHTTPS bool) (*appsv1.Deployment, error) {
	switch {
	case instance.Status.ServerConfigRef == nil:
		return nil, fmt.Errorf("CreateCTLogDeployment: %w", ServerConfigNotSpecified)
	case instance.Status.TreeID == nil:
		return nil, fmt.Errorf("CreateCTLogDeployment: %w", TreeNotSpecified)
	case instance.Spec.Trillian.Address == "":
		return nil, fmt.Errorf("CreateCTLogDeployment: %w", TrillianAddressNotSpecified)
	case instance.Spec.Trillian.Port == nil:
		return nil, fmt.Errorf("CreateCTLogDeployment: %w", TrillianPortNotSpecified)
	}
	replicas := int32(1)
	scheme := corev1.URISchemeHTTP
	if useHTTPS {
		scheme = corev1.URISchemeHTTPS
	}
	// Define a new Deployment object

	containerPorts := []corev1.ContainerPort{
		{
			ContainerPort: serverPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	appArgs := []string{
		"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(serverPort)),
		"--log_config=/ctfe-keys/config",
		"--alsologtostderr",
	}

	if instance.Spec.Monitoring.Enabled {
		appArgs = append(appArgs, "--metrics_endpoint=0.0.0.0:"+strconv.Itoa(int(metricsPort)))
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: metricsPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					Containers: []corev1.Container{
						{
							Name:  "ctlog",
							Image: constants.CTLogImage,
							Args:  appArgs,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromInt32(serverPort),
										Scheme: scheme,
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromInt32(serverPort),
										Scheme: scheme,
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "keys",
									MountPath: "/ctfe-keys",
									ReadOnly:  true,
								},
							},
							Ports: containerPorts,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "keys",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: instance.Status.ServerConfigRef.Name,
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
