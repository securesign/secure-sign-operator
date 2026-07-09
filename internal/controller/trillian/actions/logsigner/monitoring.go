package logsigner

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/utils"
)

type logsignerMonitoringConfig struct{}

func (logsignerMonitoringConfig) IsEnabled(i *rhtasv1.Trillian) bool {
	return utils.IsEnabled(i.Spec.Monitoring.ServiceMonitor.Enabled)
}

func (logsignerMonitoringConfig) TLS(i *rhtasv1.Trillian) rhtasv1.TLS {
	return statusTLS(i)
}

func NewCreateMonitorAction() action.Action[*rhtasv1.Trillian] {
	return monitoring.NewAction(
		actions.LogSignerComponentName, actions.LogSignerMonitoringName, actions.LogSignerComponentName,
		actions.ServerCondition,
		logsignerMonitoringConfig{},
	)
}
