package logsigner

import rhtasv1 "github.com/securesign/operator/api/v1"

func specTLS(instance *rhtasv1.Trillian) rhtasv1.TLS {
	return instance.Spec.LogSigner.TLS
}

func statusTLS(instance *rhtasv1.Trillian) rhtasv1.TLS {
	return instance.Status.LogSigner.TLS
}

func setStatusTLS(instance *rhtasv1.Trillian, tls rhtasv1.TLS) {
	instance.Status.LogSigner.TLS = tls
}
