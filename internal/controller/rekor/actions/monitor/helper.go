package monitor

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
)

func enabled(instance *v1alpha1.Rekor) bool {
	return utils.IsEnabled(&instance.Spec.Monitoring.TLog.Enabled) &&
		instance.Spec.Monitoring.IsServiceMonitorEnabled(kubernetes.IsOpenShift())
}
