package pkcs11

import (
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// EnsureHSMResources adds the operator-managed hsm-tokens and hsm-lib volumes
// and their corresponding mounts to the given container.
//
// hsm-tokens defaults to EmptyDir unless the caller has already provided a
// PVC-backed volume with the same name in userVolumes (matched by name).
// hsm-lib is always an operator-managed EmptyDir.
func EnsureHSMResources(podSpec *core.PodSpec, container *core.Container, userVolumes []rhtasv1.AdditionalVolume) {
	// Volume mounts on the main container.
	hsmTokensMount := kubernetes.FindVolumeMountByNameOrCreate(container, constants.HSMTokensVolumeName)
	hsmTokensMount.MountPath = constants.HSMTokensMountPath

	hsmLibMount := kubernetes.FindVolumeMountByNameOrCreate(container, constants.HSMLibVolumeName)
	hsmLibMount.MountPath = constants.HSMLibMountPath
	hsmLibMount.ReadOnly = true

	// hsm-tokens: preserve user-defined PVC or default to EmptyDir.
	hsmTokensVol := kubernetes.FindVolumeByNameOrCreate(podSpec, constants.HSMTokensVolumeName)
	userProvided := false
	for _, v := range userVolumes {
		if v.Name == constants.HSMTokensVolumeName {
			userProvided = true
			break
		}
	}
	if !userProvided {
		hsmTokensVol.VolumeSource = core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}
	}

	// hsm-lib: always operator-managed EmptyDir.
	hsmLibVol := kubernetes.FindVolumeByNameOrCreate(podSpec, constants.HSMLibVolumeName)
	hsmLibVol.VolumeSource = core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}
}

// CleanupHSMResources removes the operator-managed hsm-tokens and hsm-lib
// volumes and their corresponding mounts from the given container and pod spec.
func CleanupHSMResources(podSpec *core.PodSpec, container *core.Container) {
	kubernetes.RemoveVolumeMountByName(container, constants.HSMTokensVolumeName)
	kubernetes.RemoveVolumeMountByName(container, constants.HSMLibVolumeName)
	kubernetes.RemoveVolumeByName(podSpec, constants.HSMTokensVolumeName)
	kubernetes.RemoveVolumeByName(podSpec, constants.HSMLibVolumeName)
}

func ApplyHSMResources(podSpec *corev1ac.PodSpecApplyConfiguration, container *corev1ac.ContainerApplyConfiguration, userVolumes []rhtasv1.AdditionalVolume) {
	// Volume mounts on the main container.
	container.WithVolumeMounts(
		corev1ac.VolumeMount().
			WithName(constants.HSMTokensVolumeName).
			WithMountPath(constants.HSMTokensMountPath),
		corev1ac.VolumeMount().
			WithName(constants.HSMLibVolumeName).
			WithMountPath(constants.HSMLibMountPath).
			WithReadOnly(true),
	)

	// hsm-lib: always operator-managed EmptyDir.
	podSpec.WithVolumes(
		corev1ac.Volume().
			WithName(constants.HSMLibVolumeName).
			WithEmptyDir(corev1ac.EmptyDirVolumeSource()),
	)

	// hsm-tokens: will be overwritten by the user-provided volume if present, otherwise it will be an operator-managed EmptyDir.
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
