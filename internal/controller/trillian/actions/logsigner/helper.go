package logsigner

import "github.com/securesign/operator/api/v1alpha1"

func specTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Spec.LogSigner.TLS
}

func statusTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Status.LogSigner.TLS
}

func setStatusTLS(instance *v1alpha1.Trillian, tls v1alpha1.TLS) {
	instance.Status.LogSigner.TLS = tls
}
