package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureSecret(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, secretName string) (*corev1.Secret,
	error) {
	log := ctrllog.FromContext(ctx)
	log.Info("ensuring secret")
	// Define a new Secret object
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rhtas-" + secretName,
			Namespace: namespace,
		},
		Type: "Opaque",
		Data: map[string][]byte{
			// generate a random password for the mysql root user and the mysql password
			// TODO - use a random password generator
			"mysql-root-password": []byte("cGFzc3dvcmQ="),
			"mysql-password":      []byte("cGFzc3dvcmQ="),
			"mysql-database":      []byte("dHJpbGxpYW4="),
			"mysql-user":          []byte("bXlzcWw="),
		},
	}
	// Check if this Secret already exists else create it in the namespace
	err := r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: namespace}, secret)
	// If the Secret doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Secret")
		err = r.Create(ctx, secret)
		if err != nil {
			log.Error(err, "Failed to create new Secret")
			return nil, err
		}
	}
	return secret, nil
}
