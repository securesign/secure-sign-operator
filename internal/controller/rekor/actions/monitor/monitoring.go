package monitor

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/utils"
)

type rekorMonitorMonitoringConfig struct{}

func (rekorMonitorMonitoringConfig) IsEnabled(i *rhtasv1.Rekor) bool {
	return enabled(i) && utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}

func (rekorMonitorMonitoringConfig) TLS(_ *rhtasv1.Rekor) rhtasv1.TLS { return rhtasv1.TLS{} }

func NewCreateMonitorAction() action.Action[*rhtasv1.Rekor] {
	return monitoring.NewAction(
		actions.MonitorComponentName,
		actions.MonitoringRoleName,
		actions.MonitorStatefulSetName,
		actions.MonitorCondition,
		rekorMonitorMonitoringConfig{},
	)
}
