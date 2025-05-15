package logserver

import "github.com/securesign/operator/api/v1alpha1"

func specTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Spec.LogServer.TLS
}

func statusTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Status.LogServer.TLS
}

func setStatusTLS(instance *v1alpha1.Trillian, tls v1alpha1.TLS) {
	instance.Status.LogServer.TLS = tls
}
