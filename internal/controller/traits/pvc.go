package traits

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// PVCSpec applies the configured access modes, storage class, and storage request.
func PVCSpec(pvc rhtasv1.Pvc) Trait[*corev1ac.PersistentVolumeClaimApplyConfiguration] {
	return func(desired *corev1ac.PersistentVolumeClaimApplyConfiguration) error {
		if pvc.Size == nil {
			return fmt.Errorf("PVC size is not set")
		}
		spec := corev1ac.PersistentVolumeClaimSpec().WithResources(
			corev1ac.VolumeResourceRequirements().WithRequests(corev1.ResourceList{
				corev1.ResourceStorage: pvc.Size.DeepCopy(),
			}),
		)
		for _, mode := range pvc.AccessModes {
			spec.WithAccessModes(corev1.PersistentVolumeAccessMode(mode))
		}
		if pvc.StorageClass != "" {
			spec.WithStorageClassName(pvc.StorageClass)
		}
		desired.WithSpec(spec)
		return nil
	}
}
