package consoleUtils

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func UseTLSDb(instance *rhtasv1.Console) bool {

	if instance == nil {
		return false
	}

	// when DB is managed by operator
	if utils.IsEnabled(instance.Spec.Db.Create) && instance.Status.Db.TLS.CertRef != nil {
		return true
	}

	// external DB
	if !utils.IsEnabled(instance.Spec.Db.Create) && instance.GetTrustedCA() != nil {
		return true
	}

	return false
}

func UseTLSApi(instance *rhtasv1.Console) bool {
	if instance == nil {
		return false
	}

	return instance.Status.Api.TLS.CertRef != nil
}
