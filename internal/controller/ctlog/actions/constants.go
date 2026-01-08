package actions

import (
	"github.com/securesign/operator/internal/labels"
)

const (
	DeploymentName         = "ctlog"
	ComponentName          = "ctlog"
	RBACName               = "ctlog"
	MonitoringRoleName     = "prometheus-k8s-ctlog"
	MonitorStatefulSetName = "ctlog-monitor"
	MonitorComponentName   = "ctlog-monitor"

	CertCondition    = "FulcioCertAvailable"
	TLSCondition     = "ServerTLS"
	ConfigCondition  = "ServerConfigAvailable"
	SignerKeyReason  = "SignerKey"
	FulcioReason     = "FulcioCertificate"
	MonitorCondition = "MonitorAvailable"

	ServerPortName         = "http"
	ServerTargetPort       = 6962
	MetricsPortName        = "metrics"
	MetricsPort            = 6963
	TLSSecret              = "%s-ctlog-tls"
	MonitorMetricsPortName = "monitor-metrics"
	MonitorMetricsPort     = 9464

	CTLPubLabel       = labels.LabelNamespace + "/ctfe.pub"
	CTLogPrivateLabel = labels.LabelNamespace + "/ctfe.private"

	privateKeyRefAnnotation  = labels.LabelNamespace + "/privateKeyRef"
	passwordKeyRefAnnotation = labels.LabelNamespace + "/passwordKeyRef"
)

var (
	ManagedLabels      = []string{CTLogPrivateLabel, CTLPubLabel}
	ManagedAnnotations = []string{privateKeyRefAnnotation, passwordKeyRefAnnotation}
)
