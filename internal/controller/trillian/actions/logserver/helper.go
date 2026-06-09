package logserver

import rhtasv1 "github.com/securesign/operator/api/v1"

func specTLS(instance *rhtasv1.Trillian) rhtasv1.TLS {
	return instance.Spec.LogServer.TLS
}

func statusTLS(instance *rhtasv1.Trillian) rhtasv1.TLS {
	return instance.Status.LogServer.TLS
}

func setStatusTLS(instance *rhtasv1.Trillian, tls rhtasv1.TLS) {
	instance.Status.LogServer.TLS = tls
}
