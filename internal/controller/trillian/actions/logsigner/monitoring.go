package logsigner

import (
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/monitoring"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewCreateMonitorAction(registry *monitoring.ServiceMonitorRegistry) action.Action[*rhtasv1alpha1.Trillian] {
	return monitoring.NewMonitoringAction(monitoring.MonitoringConfig[*rhtasv1alpha1.Trillian]{
		ComponentName:      actions.LogSignerComponentName,
		DeploymentName:     actions.LogSignerComponentName,
		MonitoringRoleName: actions.LogSignerMonitoringName,
		MetricsPortName:    actions.MetricsPortName,
		IsMonitoringEnabled: func(instance *rhtasv1alpha1.Trillian) bool {
			return instance.Spec.Monitoring.Enabled
		},
		CustomEndpointBuilder: func(instance *rhtasv1alpha1.Trillian) []func(*unstructured.Unstructured) error {
			return []func(*unstructured.Unstructured) error{
				ensure.Optional(statusTLS(instance).CertRef != nil,
					kubernetes.EnsureServiceMonitorSpec(
						labels.ForComponent(actions.LogSignerComponentName, instance.Name),
						kubernetes.ServiceMonitorHttpsEndpoint(
							actions.MetricsPortName,
							fmt.Sprintf("%s.%s.svc", actions.LogsignerDeploymentName, instance.Namespace),
							statusTLS(instance).CertRef,
						),
					)),
				ensure.Optional(statusTLS(instance).CertRef == nil,
					kubernetes.EnsureServiceMonitorSpec(
						labels.ForComponent(actions.LogSignerComponentName, instance.Name),
						kubernetes.ServiceMonitorEndpoint(actions.MetricsPortName),
					)),
			}
		},
		Registry: registry,
	})
}
