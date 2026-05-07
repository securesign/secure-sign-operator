package logsigner

import (
	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
)

func specTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Spec.LogSigner.TLS
}

func statusTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Status.LogSigner.TLS
}

func setStatusTLS(instance *rhtasv1.Trillian, tls common.TLS) {
	instance.Status.LogSigner.TLS = tls
}
