package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateSecret(namespace, name, component, app string, secrets map[string]string) *corev1.Secret {
	secretData := make(map[string][]byte)
	for k, v := range secrets {
		secretData[k] = []byte(v)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": component,
				"app.kubernetes.io/name":      app,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Data: secretData,
	}
}
