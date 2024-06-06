package utils

import (
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateDeployment(instance *v1alpha1.Fulcio, deploymentName string, sa string, labels map[string]string) (*appsv1.Deployment, error) {
	if instance.Status.ServerConfigRef == nil {
		return nil, errors.New("server config ref is not specified")
	}
	if instance.Status.Certificate == nil {
		return nil, errors.New("certificate config is not specified")
	}
	if instance.Status.Certificate.PrivateKeyRef == nil {
		return nil, errors.New("private key secret is not specified")
	}

	if instance.Status.Certificate.CARef == nil {
		return nil, errors.New("CA secret is not specified")
	}

	args := []string{
		"serve",
		"--port=5555",
		"--grpc-port=5554",
		"--ca=fileca",
		"--fileca-key",
		"/var/run/fulcio-secrets/key.pem",
		"--fileca-cert",
		"/var/run/fulcio-secrets/cert.pem",
		fmt.Sprintf("--ct-log-url=http://ctlog.%s.svc/trusted-artifact-signer", instance.Namespace)}

	env := make([]corev1.EnvVar, 0)
	env = append(env, corev1.EnvVar{
		Name:  "SSL_CERT_DIR",
		Value: "/var/run/fulcio",
	})

	if instance.Status.Certificate.PrivateKeyPasswordRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: instance.Status.Certificate.PrivateKeyPasswordRef.Key,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyPasswordRef.Name,
					},
				},
			},
		})
		args = append(args, "--fileca-key-passwd", "$(PASSWORD)")
	}

	oidcInfo := make([]corev1.VolumeProjection, 0)
	// Integration with https://kubernetes.default.svc" OIDC issuer and ctlog service
	oidcInfo = append(oidcInfo, corev1.VolumeProjection{
		ConfigMap: &corev1.ConfigMapProjection{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "kube-root-ca.crt",
			},
			Items: []corev1.KeyToPath{
				{
					Key:  "ca.crt",
					Path: "ca.crt",
					Mode: utils.Pointer(int32(0444)),
				},
			},
		},
	})

	if instance.Spec.TrustedCA != nil {
		oidcInfo = append(oidcInfo, corev1.VolumeProjection{
			ConfigMap: &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: instance.Spec.TrustedCA.Name,
				},
			},
		})
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utils.Pointer(int32(1)),
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
							Name:  "fulcio-server",
							Image: constants.FulcioServerImage,
							Args:  args,
							Env:   env,
							Ports: []corev1.ContainerPort{
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5555,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 5554,
								},
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 2112,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(5555),
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
										Path: "/healthz",
										Port: intstr.FromInt32(5555),
									},
								},
								// we need to specify all defaults https://github.com/stolostron/multicluster-observability-operator/pull/301
								InitialDelaySeconds: 10,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "fulcio-config",
									MountPath: "/etc/fulcio-config",
								},
								{
									Name:      "oidc-info",
									MountPath: "/var/run/fulcio",
									ReadOnly:  true,
								},
								{
									Name:      "fulcio-cert",
									MountPath: "/var/run/fulcio-secrets",
									ReadOnly:  true,
								},
							},
						},
					},
					AutomountServiceAccountToken: &[]bool{true}[0],
					Volumes: []corev1.Volume{
						{
							Name: "fulcio-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Status.ServerConfigRef.Name,
									},
								},
							},
						},
						{
							Name: "oidc-info",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: oidcInfo,
								},
							},
						},
						{
							Name: "fulcio-cert",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: instance.Status.Certificate.PrivateKeyRef.Name,
												},
												Items: []corev1.KeyToPath{
													{
														Key:  instance.Status.Certificate.PrivateKeyRef.Key,
														Path: "key.pem",
													},
												},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: instance.Status.Certificate.CARef.Name,
												},
												Items: []corev1.KeyToPath{
													{
														Key:  instance.Status.Certificate.CARef.Key,
														Path: "cert.pem",
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
			},
		},
	}, nil
}
