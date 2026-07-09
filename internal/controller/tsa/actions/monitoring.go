package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/utils"
)

type tsaMonitoringConfig struct{}

func (tsaMonitoringConfig) IsEnabled(i *rhtasv1.TimestampAuthority) bool {
	return utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}

func (tsaMonitoringConfig) TLS(_ *rhtasv1.TimestampAuthority) rhtasv1.TLS { return rhtasv1.TLS{} }

func NewMonitoringAction() action.Action[*rhtasv1.TimestampAuthority] {
	return monitoring.NewAction(
		ComponentName,
		MonitoringRoleName,
		DeploymentName,
		"",
		tsaMonitoringConfig{},
	)
}
