package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

func TlsEnabled(instance *rhtasv1.CTlog) bool {
	return instance.Status.TLS.CertRef != nil
}
