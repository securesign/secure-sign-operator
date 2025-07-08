package server

import (
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabledFileAttestationStorage(instance *rhtasv1alpha1.Rekor) bool {
	return utils.IsEnabled(instance.Spec.Attestations.Enabled) &&
		(strings.HasPrefix(instance.Spec.Attestations.Url, "file://") || instance.Spec.Attestations.Url == "")
}
