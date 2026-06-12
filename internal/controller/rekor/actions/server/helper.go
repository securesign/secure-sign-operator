package server

import (
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabledFileAttestationStorage(instance *rhtasv1.Rekor) bool {
	return utils.IsEnabled(instance.Spec.Attestations.Enabled) &&
		(strings.HasPrefix(instance.Spec.Attestations.Url, "file://") || instance.Spec.Attestations.Url == "")
}
