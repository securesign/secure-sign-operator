package utils

import (
	"errors"

	"github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func SetTLS(template *corev1.PodTemplateSpec, tls v1alpha1.TLS) error {
	if template == nil {
		return errors.New("SetTLS: PodTemplateSpec is not set")
	}
	if tls.CertRef == nil {
		return nil
	}

	template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
		Name: "tls-cert",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: tls.CertRef.Name,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  tls.CertRef.Key,
									Path: "tls.crt",
								},
							},
						},
					},
					{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: tls.PrivateKeyRef.Name,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  tls.PrivateKeyRef.Key,
									Path: "tls.key",
								},
							},
						},
					},
				},
			},
		},
	})

	for i := range template.Spec.Containers {
		template.Spec.Containers[i].VolumeMounts = append(template.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/var/run/secrets/tas",
				ReadOnly:  true,
			})
	}

	return nil
}
