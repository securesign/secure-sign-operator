package utils

import "github.com/securesign/operator/api/v1alpha1"

func TlsEnabled(instance *v1alpha1.CTlog) bool {
	return instance.Status.TLS.CertRef != nil
}
