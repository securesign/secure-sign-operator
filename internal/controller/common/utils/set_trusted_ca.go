package utils

import (
	"errors"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	corev1 "k8s.io/api/core/v1"
)

// SetTrustedCA mount config map with trusted CA bundle to all deployment's containers.
func SetTrustedCA(template *corev1.PodTemplateSpec, lor *v1alpha1.LocalObjectReference) error {
	if template == nil {
		return errors.New("SetTrustedCA: PodTemplateSpec is not set")
	}

	for i, container := range template.Spec.Containers {
		if template.Spec.Containers[i].Env == nil {
			template.Spec.Containers[i].Env = make([]corev1.EnvVar, 0)
		}
		template.Spec.Containers[i].Env = append(container.Env, corev1.EnvVar{
			Name:  "SSL_CERT_DIR",
			Value: "/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount",
		})

		if template.Spec.Containers[i].VolumeMounts == nil {
			template.Spec.Containers[i].VolumeMounts = make([]corev1.VolumeMount, 0)
		}
		template.Spec.Containers[i].VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "ca-trust",
			MountPath: "/var/run/configs/tas/ca-trust",
			ReadOnly:  true,
		})
	}

	projections := make([]corev1.VolumeProjection, 0)
	if lor != nil {
		projections = append(projections, corev1.VolumeProjection{
			ConfigMap: &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: lor.Name,
				},
			},
		})
	}

	if template.Spec.Volumes == nil {
		template.Spec.Volumes = make([]corev1.Volume, 0)
	}
	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
		Name: "ca-trust",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources:     projections,
				DefaultMode: Pointer(int32(420)),
			},
		},
	})
	return nil
}

func TrustedCAAnnotationToReference(anns map[string]string) *v1alpha1.LocalObjectReference {
	if v, ok := anns[annotations.TrustedCA]; ok {
		return &v1alpha1.LocalObjectReference{
			Name: v,
		}
	}
	return nil
}
