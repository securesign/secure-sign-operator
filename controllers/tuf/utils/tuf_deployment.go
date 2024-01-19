package utils

import (
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTufDeployment(namespace string, dpName string, fulcioSecret string, rekorSecret string, labels map[string]string) *apps.Deployment {
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
					ServiceAccountName: constants.ServiceAccountName,
					Volumes: []core.Volume{
						{
							Name: "tuf-secrets",
							VolumeSource: core.VolumeSource{
								Projected: &core.ProjectedVolumeSource{
									Sources: []core.VolumeProjection{
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: "ctlog-public-key",
												},
												Items: []core.KeyToPath{
													{
														Key:  "public",
														Path: "ctfe.pub",
													},
												},
											},
										},
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: fulcioSecret,
												},
												Items: []core.KeyToPath{
													{
														Key:  "cert",
														Path: "fulcio-cert",
													},
												},
											},
										},
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: rekorSecret,
												},
												Items: []core.KeyToPath{
													{
														Key:  "key",
														Path: "rekor-pubkey",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: constants.TufImage,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "tuf-secrets",
									MountPath: "/var/run/tuf-secrets",
								},
							},
						},
					},
				},
			},
		},
	}
}
