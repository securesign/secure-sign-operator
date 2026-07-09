package monitor

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/utils"
)

type ctlogMonitorMonitoringConfig struct{}

func (ctlogMonitorMonitoringConfig) IsEnabled(i *rhtasv1.CTlog) bool {
	return enabled(i) && utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}

func (ctlogMonitorMonitoringConfig) TLS(_ *rhtasv1.CTlog) rhtasv1.TLS { return rhtasv1.TLS{} }

func NewCreateMonitorAction() action.Action[*rhtasv1.CTlog] {
	return monitoring.NewAction(
		actions.MonitorComponentName,
		actions.MonitoringRoleName,
		actions.MonitorStatefulSetName,
		actions.MonitorCondition,
		ctlogMonitorMonitoringConfig{},
	)
}
