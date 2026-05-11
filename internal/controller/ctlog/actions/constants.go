package actions

import (
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/labels"

	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
)

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

func resolveTrillianAddress(instance *rhtasv1alpha1.CTlog) string {
	if instance.Spec.Trillian.Address != "" {
		return instance.Spec.Trillian.Address
	}
	return fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
}
