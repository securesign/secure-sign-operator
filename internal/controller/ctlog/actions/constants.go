package actions

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	CertCondition       = "FulcioCertAvailable"
	ServerPortName      = "http"
	ServerPort          = 80
	HttpsServerPortName = "https"
	HttpsServerPort     = 443
	ServerTargetPort    = 6962
	MetricsPortName     = "metrics"
	MetricsPort         = 6963
	ServerCondition     = "ServerAvailable"
)
