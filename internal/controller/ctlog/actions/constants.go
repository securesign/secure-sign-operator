package actions

import (
	"github.com/securesign/operator/internal/controller/labels"
)

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	CertCondition = "FulcioCertAvailable"

	ConfigCondition    = "ServerConfigAvailable"
	TrillianTreeReason = "TrillianTree"
	SignerKeyReason    = "SignerKey"
	FulcioReason       = "FulcioCertificate"

	ServerPortName   = "http"
	ServerPort       = 80
	ServerTargetPort = 6962
	MetricsPortName  = "metrics"
	MetricsPort      = 6963
	ServerCondition  = "ServerAvailable"

	CTLPubLabel       = labels.LabelNamespace + "/ctfe.pub"
	CTLogPrivateLabel = labels.LabelNamespace + "/ctfe.private"

	privateKeyRefAnnotation  = labels.LabelNamespace + "/privateKeyRef"
	passwordKeyRefAnnotation = labels.LabelNamespace + "/passwordKeyRef"
)
