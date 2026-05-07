package actions

import (
	"github.com/securesign/operator/api/common"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *rhtasv1.Rekor) bool {
	return utils.OptionalBool(instance.Spec.SearchIndex.Create)
}

func specTLS(instance *rhtasv1.Rekor) common.TLS {
	return instance.Spec.SearchIndex.TLS
}
func statusTLS(instance *rhtasv1.Rekor) common.TLS {
	return instance.Status.SearchIndex.TLS
}

func setStatusTLS(instance *rhtasv1.Rekor, tls common.TLS) {
	instance.Status.SearchIndex.TLS = tls
}
