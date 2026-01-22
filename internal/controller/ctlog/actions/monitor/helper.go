package monitor

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.CTlog) bool {
	return utils.IsEnabled(&instance.Spec.Monitoring.TLog.Enabled)
}
