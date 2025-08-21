package kubernetes

import (
	"github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func EnsurePVCSpec(instancePvc v1alpha1.Pvc) func(pvc *corev1.PersistentVolumeClaim) error {
	return func(pvc *corev1.PersistentVolumeClaim) error {
		spec := &pvc.Spec

		// immutable for bound claims
		if len(spec.AccessModes) == 0 {
			modes := make([]corev1.PersistentVolumeAccessMode, len(instancePvc.AccessModes))
			for i, m := range instancePvc.AccessModes {
				modes[i] = corev1.PersistentVolumeAccessMode(m)
			}
			spec.AccessModes = modes
		}

		// immutable volumeAttributesClassName for bound claims
		if spec.StorageClassName == nil {
			if instancePvc.StorageClass != "" {
				spec.StorageClassName = &instancePvc.StorageClass
			}
		}

		spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: *instancePvc.Size,
			},
		}

		return nil
	}
}
