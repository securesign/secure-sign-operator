package ui

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *rhtasv1.Rekor) bool {
	return utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled)
}
