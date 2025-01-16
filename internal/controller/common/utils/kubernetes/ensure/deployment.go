package ensure

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func Proxy() func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		utils.SetProxyEnvs(dp)
		return nil
	}
}

const CaTrustVolumeName = "ca-trust"

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		template := &dp.Spec.Template
		for i := range template.Spec.Containers {
			env := kubernetes.FindEnvByNameOrCreate(&template.Spec.Containers[i], "SSL_CERT_DIR")
			env.Value = "/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount"

			volumeMount := kubernetes.FindVolumeMountByNameOrCreate(&template.Spec.Containers[i], CaTrustVolumeName)
			volumeMount.MountPath = "/var/run/configs/tas/ca-trust"
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
