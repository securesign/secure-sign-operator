package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreatePVC(namespace string, pvcName string, pvcSize resource.Quantity, storageClass string, labels map[string]string) *corev1.PersistentVolumeClaim {
	var computedStorageClass *string
	if storageClass == "" {
		computedStorageClass = nil
	} else {
		computedStorageClass = &storageClass
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				"ReadWriteOnce",
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): pvcSize,
				},
			},
			StorageClassName: computedStorageClass,
		},
	}
}

func GetPVC(ctx context.Context, c client.Client, namespace, pvcName string) (bool, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: pvcName}, pvc)
	if err == nil {
		// PVC exists
		return true, nil
	} else if errors.IsNotFound(err) {
		// PVC does not exist
		return false, nil
	} else {
		// Error while checking for PVC existence
		return false, err
	}
}
