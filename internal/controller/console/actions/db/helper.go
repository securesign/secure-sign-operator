package db

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.Console) bool {
	return utils.OptionalBool(instance.Spec.Db.Create) && instance.Spec.Enabled
}

func specTLS(instance *v1alpha1.Console) v1alpha1.TLS {
	return instance.Spec.Db.TLS
}
func statusTLS(instance *v1alpha1.Console) v1alpha1.TLS {
	return instance.Status.Db.TLS
}

func setStatusTLS(instance *v1alpha1.Console, tls v1alpha1.TLS) {
	instance.Status.Db.TLS = tls
}
