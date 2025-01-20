package ensure

import (
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	CaTrustVolumeName = "ca-trust"
	TLSVolumeName     = "tls-cert"
	CATRustMountPath  = "/var/run/configs/tas/ca-trust"

	TLSVolumeMount = "/var/run/secrets/tas"

	TLSKeyPath  = TLSVolumeMount + "/tls.key"
	TLSCertPath = TLSVolumeMount + "/tls.crt"
)

func Proxy() func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		utils.SetProxyEnvs(dp)
		return nil
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		template := &dp.Spec.Template
		for i := range template.Spec.Containers {
			env := kubernetes.FindEnvByNameOrCreate(&template.Spec.Containers[i], "SSL_CERT_DIR")
			env.Value = CATRustMountPath + ":/var/run/secrets/kubernetes.io/serviceaccount"

			volumeMount := kubernetes.FindVolumeMountByNameOrCreate(&template.Spec.Containers[i], CaTrustVolumeName)
			volumeMount.MountPath = CATRustMountPath
			volumeMount.ReadOnly = true

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

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, CaTrustVolumeName)
		if volume.Projected == nil {
			volume.Projected = &corev1.ProjectedVolumeSource{}
		}
		volume.Projected.Sources = projections

		return nil
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls v1alpha1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		template := &dp.Spec.Template

		for i, c := range template.Spec.Containers {
			if slices.Contains(containerNames, c.Name) {
				volumeMount := kubernetes.FindVolumeMountByNameOrCreate(&template.Spec.Containers[i], TLSVolumeName)
				volumeMount.MountPath = TLSVolumeMount
				volumeMount.ReadOnly = true
			}
		}

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, TLSVolumeName)
		if volume.Projected == nil {
			volume.Projected = &corev1.ProjectedVolumeSource{}
		}
		volume.Projected.Sources = []corev1.VolumeProjection{
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
		}
		return nil
	}
}
