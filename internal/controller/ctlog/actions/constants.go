package actions

import "github.com/securesign/operator/internal/controller/constants"

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	CertCondition             = "FulcioCertAvailable"
	ServerPortName            = "http"
	ServerPort                = 80
	HttpsServerPortName       = "https"
	HttpsServerPort           = 443
	ServerTargetPort          = 6962
	MetricsPortName           = "metrics"
	MetricsPort               = 6963
	ServerCondition           = "ServerAvailable"
	CtlogTreeName             = "ctlog-tree"
	CtlogTreeJobName          = "ctlog-create-tree"
	CtlogTreeJobCondition     = "CtlogTreeJobAvailable"
	CtlogTreeJobConfigMapName = "ctlog-tree-id-config"

	CTLPubLabel = constants.LabelNamespace + "/ctfe.pub"
)
