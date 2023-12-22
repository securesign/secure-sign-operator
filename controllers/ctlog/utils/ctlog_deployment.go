package utils

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateDeployment(namespace string, deploymentName string, ssapp string) *appsv1.Deployment {

	replicas := int32(1)
	// Define a new Deployment object
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      ssapp,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/component": ssapp,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      ssapp,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
					"app.kubernetes.io/component": ssapp},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      ssapp,
						"app.kubernetes.io/component": ssapp,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "sigstore-sa",
					Containers: []corev1.Container{
						{
							Name:  "ctlog",
							Image: "registry.redhat.io/rhtas-tech-preview/ct-server-rhel9@sha256:6124a531097c91bf8c872393a6f313c035ca03eca316becd3c350930d978929f",
							Args: []string{
								"--http_endpoint=0.0.0.0:6962",
								"--metrics_endpoint=0.0.0.0:6963",
								"--log_config=/ctfe-keys/config",
								"--alsologtostderr",
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(6962),
									},
								},
								InitialDelaySeconds: 10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(6962),
									},
								},
								InitialDelaySeconds: 10,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "keys",
									MountPath: "/ctfe-keys",
									ReadOnly:  true,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6962,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: 6963,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "keys",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "ctlog-secret",
								},
							},
						},
					},
				},
			},
		},
	}
}
