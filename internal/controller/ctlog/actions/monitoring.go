package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
)

func NewCreateMonitorAction(registry *monitoring.ServiceMonitorRegistry) action.Action[*rhtasv1alpha1.CTlog] {
	return monitoring.NewMonitoringAction(monitoring.MonitoringConfig[*rhtasv1alpha1.CTlog]{
		ComponentName:      ComponentName,
		DeploymentName:     DeploymentName,
		MonitoringRoleName: MonitoringRoleName,
		MetricsPortName:    MetricsPortName,
		IsMonitoringEnabled: func(instance *rhtasv1alpha1.CTlog) bool {
			return instance.Spec.Monitoring.Enabled
		},
		Registry: registry,
	})
}
