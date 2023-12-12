package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureSA(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, svcAccount string) (*corev1.ServiceAccount,
	error) {
	log := ctrllog.FromContext(ctx)
	log.Info("ensuring service account")
	// Define a new Service Account object
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rhtas-" + svcAccount,
			Namespace: namespace,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "pull-secret",
			},
		},
	}
	// Check if this service account already exists else create it in the namespace
	err := r.Get(ctx, client.ObjectKey{Name: sa.Name, Namespace: namespace}, sa)
	// If the SA doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Service Account")
		err = r.Create(ctx, sa)
		if err != nil {
			log.Error(err, "Failed to create new service account")
			return nil, err
		}
	}
	return sa, nil
}
