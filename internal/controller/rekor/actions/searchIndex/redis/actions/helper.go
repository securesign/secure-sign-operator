package actions

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.Rekor) bool {
	return utils.OptionalBool(instance.Spec.SearchIndex.Create)
}

func specTLS(instance *v1alpha1.Rekor) v1alpha1.TLS {
	return instance.Spec.SearchIndex.TLS
}
func statusTLS(instance *v1alpha1.Rekor) v1alpha1.TLS {
	return instance.Status.SearchIndex.TLS
}

func setStatusTLS(instance *v1alpha1.Rekor, tls v1alpha1.TLS) {
	instance.Status.SearchIndex.TLS = tls
}
