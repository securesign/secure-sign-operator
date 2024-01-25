package utils

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func secretsVolumeProjection(spec v1alpha1.TufSpec) *core.ProjectedVolumeSource {

	projections := make([]core.VolumeProjection, 0)

	for _, key := range spec.Keys {
		p := core.VolumeProjection{Secret: selectorToProjection(key.SecretRef, key.Name)}
		projections = append(projections, p)
	}

	return &core.ProjectedVolumeSource{
		Sources: projections,
	}
}

func selectorToProjection(secret *v1alpha1.SecretKeySelector, path string) *core.SecretProjection {
	return &core.SecretProjection{
		LocalObjectReference: core.LocalObjectReference{
			Name: secret.Name,
		},
		Items: []core.KeyToPath{
			{
				Key:  secret.Key,
				Path: path,
			},
		},
	}
}

func CreateTufDeployment(instance *v1alpha1.Tuf, dpName string, labels map[string]string, serviceAccountName string) *apps.Deployment {
	replicas := int32(1)
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
					ServiceAccountName: serviceAccountName,
					Volumes: []core.Volume{
						{
							Name: "tuf-secrets",
							VolumeSource: core.VolumeSource{
								Projected: secretsVolumeProjection(instance.Spec),
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  "tuf",
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
									Value: instance.Namespace,
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
