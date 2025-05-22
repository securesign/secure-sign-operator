package kubernetes

import (
	"context"

	"github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPVC(ctx context.Context, c client.Client, namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: pvcName}, pvc)
	return pvc, err
}

func EnsurePVCSpec(instancePvc v1alpha1.Pvc) func(pvc *corev1.PersistentVolumeClaim) error {
	return func(pvc *corev1.PersistentVolumeClaim) error {
		modes := make([]corev1.PersistentVolumeAccessMode, len(instancePvc.AccessModes))
		for i, m := range instancePvc.AccessModes {
			modes[i] = corev1.PersistentVolumeAccessMode(m)
		}
		var computedStorageClass *string
		if instancePvc.StorageClass == "" {
			computedStorageClass = nil
		} else {
			computedStorageClass = &instancePvc.StorageClass
		}

		spec := &pvc.Spec

		spec.AccessModes = modes
		spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: *instancePvc.Size,
			}}
		spec.StorageClassName = computedStorageClass
		return nil
	}
}
