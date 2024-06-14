package actions

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	ServerPortName   = "http"
	ServerPort       = 80
	ServerTargetPort = 6962
	MetricsPortName  = "metrics"
	MetricsPort      = 6963
	KeyCondition     = "KeyAvailable"
	CertCondition    = "FulcioCertAvailable"
	ServerCondition  = "ServerAvailable"
)
