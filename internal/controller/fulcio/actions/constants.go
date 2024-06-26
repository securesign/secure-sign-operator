package actions

const (
	DeploymentName     = "fulcio-server"
	ComponentName      = "fulcio"
	MonitoringRoleName = "prometheus-k8s-fulcio"
	ServiceMonitorName = "fulcio-metrics"
	RBACName           = "fulcio"

	CertCondition = "FulcioCertAvailable"

	PortName = "metrics"
	Port     = 2112
)
