package monitor

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *rhtasv1.CTlog) bool {
	return utils.IsEnabled(instance.Spec.Monitoring.TLog.Enabled)
}
