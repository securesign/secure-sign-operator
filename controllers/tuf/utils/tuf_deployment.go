package utils

import (
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTufDeployment(namespace string, image string, dpName string) *apps.Deployment {
	replicas := int32(1)
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": dpName,
				"app.kubernetes.io/name":      dpName,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": dpName,
					"app.kubernetes.io/name":      dpName,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": dpName,
						"app.kubernetes.io/name":      dpName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: "sigstore-sa",
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
													Name: "fulcio-secret-rh",
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
													Name: "rekor-public-key",
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
							Image: image,
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
