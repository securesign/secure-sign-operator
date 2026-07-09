package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/utils"
)

type ctlogMonitoringConfig struct{}

func (ctlogMonitoringConfig) IsEnabled(i *rhtasv1.CTlog) bool {
	return utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}

func (ctlogMonitoringConfig) TLS(_ *rhtasv1.CTlog) rhtasv1.TLS { return rhtasv1.TLS{} }

func NewCreateMonitorAction() action.Action[*rhtasv1.CTlog] {
	return monitoring.NewAction(
		ComponentName,
		MonitoringRoleName,
		DeploymentName,
		"",
		ctlogMonitoringConfig{},
	)
}
