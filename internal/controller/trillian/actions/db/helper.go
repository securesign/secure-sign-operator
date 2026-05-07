package db

import (
	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *rhtasv1.Trillian) bool {
	return utils.OptionalBool(instance.Spec.Db.Create)
}

func specTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Spec.Db.TLS
}
func statusTLS(instance *rhtasv1.Trillian) common.TLS {
	return instance.Status.Db.TLS
}

func setStatusTLS(instance *rhtasv1.Trillian, tls common.TLS) {
	instance.Status.Db.TLS = tls
}
