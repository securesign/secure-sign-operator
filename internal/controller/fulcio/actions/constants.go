package actions

import "github.com/securesign/operator/internal/labels"

const (
	FulcioCALabel = labels.LabelNamespace + "/fulcio_v1.crt.pem"

	DeploymentName     = "fulcio-server"
	ComponentName      = "fulcio"
	MonitoringRoleName = "prometheus-k8s-fulcio"
	ServiceMonitorName = "fulcio-metrics"
	RBACName           = "fulcio"

	CertCondition   = "FulcioCertAvailable"
	PKCS11Condition = "FulcioPKCS11ConfigAvailable"

	PKCS11CertMountPath    = "/var/run/fulcio-pkcs11-cert"
	PKCS11CertVolumeName   = "fulcio-pkcs11-cert"
	PKCS11ConfigMountPath  = "/var/run/pkcs11-config"
	PKCS11ConfigVolumeName = "pkcs11-config"

	ServerPortName   = "http"
	ServerPort       = 80
	TargetServerPort = 5555
	GRPCPortName     = "grpc"
	GRPCPort         = 5554
	MetricsPortName  = "metrics"
	MetricsPort      = 2112
)
