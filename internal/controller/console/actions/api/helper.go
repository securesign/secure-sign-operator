package api

import "github.com/securesign/operator/api/v1alpha1"

func specTLS(instance *v1alpha1.Console) v1alpha1.TLS {
	return instance.Spec.Api.TLS
}

func statusTLS(instance *v1alpha1.Console) v1alpha1.TLS {
	return instance.Status.Api.TLS
}

func setStatusTLS(instance *v1alpha1.Console, tls v1alpha1.TLS) {
	instance.Status.Api.TLS = tls
}
