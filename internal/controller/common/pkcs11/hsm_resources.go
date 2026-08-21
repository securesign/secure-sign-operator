package pkcs11

import (
	"fmt"
	"hash"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
)

// HashCoreConfig writes the shared PKCS11Config fields into the given hash.
// Callers add their component-specific fields before/after and finalize the hash.
func HashCoreConfig(h hash.Hash, cfg *rhtasv1.PKCS11Config) {
	if cfg == nil {
		return
	}
	fmt.Fprintf(h, "modulePath:%s\n", cfg.ModulePath) //nolint:errcheck // hash.Hash.Write never returns an error
	fmt.Fprintf(h, "tokenLabel:%s\n", cfg.TokenLabel) //nolint:errcheck
	if cfg.PinSecretRef != nil {
		fmt.Fprintf(h, "pinSecretRef:%s/%s\n", cfg.PinSecretRef.Name, cfg.PinSecretRef.Key) //nolint:errcheck
	}
	keyID := int32(0)
	if cfg.KeyID != nil {
		keyID = *cfg.KeyID
	}
	fmt.Fprintf(h, "keyConfig:%d/%s\n", keyID, cfg.KeyLabel) //nolint:errcheck
}

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
