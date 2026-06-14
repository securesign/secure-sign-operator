package api

import rhtasv1 "github.com/securesign/operator/api/v1"

func specTLS(instance *rhtasv1.Console) rhtasv1.TLS {
	return instance.Spec.Api.TLS
}

func statusTLS(instance *rhtasv1.Console) rhtasv1.TLS {
	return instance.Status.Api.TLS
}

func setStatusTLS(instance *rhtasv1.Console, tls rhtasv1.TLS) {
	instance.Status.Api.TLS = tls
}
