package logserver

import (
	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
)

func specTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Spec.LogServer.TLS
}

func statusTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Status.LogServer.TLS
}

func setStatusTLS(instance *rhtasv1.Trillian, tls common.TLS) {
	instance.Status.LogServer.TLS = tls
}
