package pkcs11

import (
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func ApplyHSMResources(podSpec *corev1ac.PodSpecApplyConfiguration, container *corev1ac.ContainerApplyConfiguration, userVolumes []rhtasv1.AdditionalVolume) {
	container.WithVolumeMounts(
		corev1ac.VolumeMount().
			WithName(constants.HSMTokensVolumeName).
			WithMountPath(constants.HSMTokensMountPath),
		corev1ac.VolumeMount().
			WithName(constants.HSMLibVolumeName).
			WithMountPath(constants.HSMLibMountPath).
			WithReadOnly(true),
	)

	podSpec.WithVolumes(
		corev1ac.Volume().
			WithName(constants.HSMLibVolumeName).
			WithEmptyDir(corev1ac.EmptyDirVolumeSource()),
	)

	if !slices.ContainsFunc(userVolumes, func(v rhtasv1.AdditionalVolume) bool {
		return v.Name == constants.HSMTokensVolumeName
	}) {
		podSpec.WithVolumes(
			corev1ac.Volume().
				WithName(constants.HSMTokensVolumeName).
				WithEmptyDir(corev1ac.EmptyDirVolumeSource()),
		)
	}
}
