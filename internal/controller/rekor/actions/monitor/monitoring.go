package monitor

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

func NewCreateMonitorAction(registry *monitoring.ServiceMonitorRegistry) action.Action[*rhtasv1alpha1.Rekor] {
	return monitoring.NewMonitoringAction(monitoring.MonitoringConfig[*rhtasv1alpha1.Rekor]{
		ComponentName:       actions.MonitorComponentName,
		DeploymentName:      actions.MonitorStatefulSetName,
		MonitoringRoleName:  actions.MonitoringRoleName,
		MetricsPortName:     actions.MonitorMetricsPortName,
		IsMonitoringEnabled: enabled,
		Registry:            registry,
	})
}
