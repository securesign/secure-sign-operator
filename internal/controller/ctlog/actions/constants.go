package actions

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"
	ServerCondition    = "ServerAvailable"

	CertCondition = "FulcioCertAvailable"

	PortName        = "http"
	Port            = 80
	MetricsPortName = "metrics"
	MetricsPort     = 6963
)
