package actions

const (
	DeploymentName         = "ctlog"
	ComponentName          = "ctlog"
	RBACName               = "ctlog"
	RBACMonitorName        = "ctlog-monitor"
	MonitoringRoleName     = "prometheus-k8s-ctlog"
	MonitorStatefulSetName = "ctlog-monitor"
	MonitorComponentName   = "ctlog-monitor"

	CertCondition    = "FulcioCertAvailable"
	TLSCondition     = "ServerTLS"
	ConfigCondition  = "ServerConfigAvailable"
	SignerCondition  = "SignerAvailable"
	SignerKeyReason  = "SignerKey"
	FulcioReason     = "FulcioCertificate"
	MonitorCondition = "MonitorAvailable"

	// PKCS11Condition tracks PKCS#11 configuration readiness.
	PKCS11Condition = "PKCS11ConfigAvailable"

	ServerPortName         = "http"
	ServerTargetPort       = 6962
	MetricsPortName        = "metrics"
	MetricsPort            = 6963
	TLSSecret              = "%s-ctlog-tls"
	MonitorMetricsPortName = "monitor-metrics"
	MonitorMetricsPort     = 9464

	// PKCS#11 init container and volume names
	HSMInitContainerName      = "hsm-init"
	HSMLibExportContainerName = "hsm-lib-export"
	HSMTokensVolumeName       = "hsm-tokens"
	HSMLibVolumeName          = "hsm-lib"
	HSMTokenMountPath         = "/var/lib/hsm/tokens"
	HSMLibMountPath           = "/var/lib/hsm/lib"
	HSMPinEnvVar              = "HSM_PIN"
)
