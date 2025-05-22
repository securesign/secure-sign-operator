package db

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.Trillian) bool {
	return utils.OptionalBool(instance.Spec.Db.Create)
}

func specTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Spec.Db.TLS
}
func statusTLS(instance *v1alpha1.Trillian) v1alpha1.TLS {
	return instance.Status.Db.TLS
}

func setStatusTLS(instance *v1alpha1.Trillian, tls v1alpha1.TLS) {
	instance.Status.Db.TLS = tls
}
