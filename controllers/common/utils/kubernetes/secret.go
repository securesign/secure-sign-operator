package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateSecret(name string, namespace string, data map[string][]byte, labels map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}
}

func GetSecret(client client.Client, namespace, secretName string) (*corev1.Secret, error) {
	var secret corev1.Secret

	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, &secret)

	if err != nil {
		return nil, err
	}
	return &secret, nil
}
