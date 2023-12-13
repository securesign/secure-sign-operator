package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensurePVC(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, pvcName string) (*corev1.PersistentVolumeClaim,
	error) {
	log := ctrllog.FromContext(ctx)
	log.Info("ensuring pvc")
	pvcSize := "5Gi"
	// Define a new PVC object
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				"ReadWriteOnce",
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(pvcSize),
				},
			},
		},
	}
	// Check if this PVC already exists else create it in the namespace
	err := r.Get(ctx, client.ObjectKey{Name: pvc.Name, Namespace: namespace}, pvc)
	// If the PVC doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new PVC")
		err = r.Create(ctx, pvc)
		if err != nil {
			log.Error(err, "Failed to create new PVC")
			return nil, err
		}
	}
	return pvc, nil
}
