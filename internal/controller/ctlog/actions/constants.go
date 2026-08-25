package actions

const (
	DeploymentName         = "ctlog"
	ComponentName          = "ctlog"
	RBACName               = "ctlog"
	RBACMonitorName        = "ctlog-monitor"
	MonitoringRoleName     = "prometheus-k8s-ctlog"
	MonitorStatefulSetName = "ctlog-monitor"
	MonitorComponentName   = "ctlog-monitor"

	CertCondition         = "FulcioCertAvailable"
	TLSCondition          = "ServerTLS"
	ConfigCondition       = "ServerConfigAvailable"
	SignerCondition       = "SignerAvailable"
	PKCS11Condition       = "PKCS11ConfigAvailable"
	PKCS11MessageResolved = "PKCS#11 config resolved"

	ReasonResolved   = "Resolved"
	SignerKeyReason  = "SignerKey"
	FulcioReason     = "FulcioCertificate"
	MonitorCondition = "MonitorAvailable"

	ServerPortName     = "http"
	ServerTargetPort   = 6962
	MetricsPortName    = "metrics"
	MetricsPort        = 6963
	TLSSecret          = "%s-ctlog-tls"
	MonitorMetricsPort = 9464
)
