package controllers

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureNamespace(ctx context.Context, m *rhtasv1alpha1.Securesign, component string) (*corev1.Namespace, error) {
	log := ctrllog.FromContext(ctx)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name + "-" + component,
		},
	}

	// Check if this Namespace already exists else create it
	err := r.Get(ctx, client.ObjectKey{Name: ns.Name}, ns)
	// If the Namespace doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Namespace")
		err = r.Create(ctx, ns)
		if err != nil {
			log.Error(err, "Failed to create new Namespace")
			return nil, err
		}
	}
	return ns, nil
}
