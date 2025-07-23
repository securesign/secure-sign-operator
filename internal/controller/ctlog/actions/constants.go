package actions

import (
	"github.com/securesign/operator/internal/labels"
)

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	CertCondition   = "FulcioCertAvailable"
	TLSCondition    = "ServerTLS"
	ConfigCondition = "ServerConfigAvailable"
	SignerKeyReason = "SignerKey"
	FulcioReason    = "FulcioCertificate"

	ServerPortName   = "http"
	ServerTargetPort = 6962
	MetricsPortName  = "metrics"
	MetricsPort      = 6963
	TLSSecret        = "%s-ctlog-tls"

	CTLPubLabel       = labels.LabelNamespace + "/ctfe.pub"
	CTLogPrivateLabel = labels.LabelNamespace + "/ctfe.private"

	privateKeyRefAnnotation  = labels.LabelNamespace + "/privateKeyRef"
	passwordKeyRefAnnotation = labels.LabelNamespace + "/passwordKeyRef"

	// maxCertificateSize limits certificate size to 50 KiB
	maxCertificateSize = 50 * 1024
)

var (
	ManagedLabels      = []string{CTLogPrivateLabel, CTLPubLabel}
	ManagedAnnotations = []string{privateKeyRefAnnotation, passwordKeyRefAnnotation}
)
