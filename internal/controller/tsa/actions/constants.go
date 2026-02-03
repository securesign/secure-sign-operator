package actions

const (
	DeploymentName     = "tsa-server"
	ComponentName      = "timestamp-authority"
	RBACName           = "tsa"
	MonitoringRoleName = "prometheus-k8s-tsa"
	TSASignerCondition = "TSASignerCondition"
	ServerPortName     = "tsa-server"
	ServerPort         = 3000
	MetricsPortName    = "metrics"
	MetricsPort        = 2112
	NtpCMName          = "ntp-config-"
	TimestampPath      = "/api/v1/timestamp"
)
