package actions

const (
	DeploymentName     = "fulcio-server"
	ComponentName      = "fulcio"
	MonitoringRoleName = "prometheus-k8s-fulcio"
	ServiceMonitorName = "fulcio-metrics"
	RBACName           = "fulcio"

	CertCondition = "FulcioCertAvailable"

	ServerPortName   = "http"
	ServerPort       = 80
	TargetServerPort = 5555
	GRPCPortName     = "grpc"
	GRPCPort         = 5554
	MetricsPortName  = "metrics"
	MetricsPort      = 2112

	HSMInitContainerName        = "hsm-init"
	HSMLibExportContainerName   = "hsm-lib-export"
	FulcioCreateCAContainerName = "fulcio-createca"
	HSMTokenMountPath           = "/var/lib/hsm/tokens"
	HSMLibMountPath             = "/var/lib/hsm/lib"
	PKCS11ConfigMountPath       = "/etc/fulcio-pkcs11"
	PKCS11RootPEMPath           = "/var/lib/hsm/lib/fulcio-root.pem"
)
