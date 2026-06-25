package db

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *rhtasv1.Console) bool {
	return utils.OptionalBool(instance.Spec.Db.Create) && instance.Spec.Enabled
}

func specTLS(instance *rhtasv1.Console) rhtasv1.TLS {
	return instance.Spec.Db.TLS
}
func statusTLS(instance *rhtasv1.Console) rhtasv1.TLS {
	return instance.Status.Db.TLS
}

func setStatusTLS(instance *rhtasv1.Console, tls rhtasv1.TLS) {
	instance.Status.Db.TLS = tls
}
