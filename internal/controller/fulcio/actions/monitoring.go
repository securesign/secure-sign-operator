package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/utils"
)

type fulcioMonitoringConfig struct{}

func (fulcioMonitoringConfig) IsEnabled(i *rhtasv1.Fulcio) bool {
	return utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}
func (fulcioMonitoringConfig) TLS(_ *rhtasv1.Fulcio) rhtasv1.TLS { return rhtasv1.TLS{} }

func NewCreateMonitorAction() action.Action[*rhtasv1.Fulcio] {
	return monitoring.NewAction(
		ComponentName,
		MonitoringRoleName,
		DeploymentName,
		"",
		fulcioMonitoringConfig{},
	)
}
