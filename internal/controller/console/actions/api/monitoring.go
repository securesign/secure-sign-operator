package api

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewCreateMonitorAction() action.Action[*rhtasv1alpha1.Console] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Console) bool {
	return instance.Spec.Enabled && instance.Spec.Monitoring.Enabled &&
		state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {
	var (
		err error
	)

	monitoringLabels := labels.For(actions.ApiComponentName, actions.ApiMonitoringName, instance.Name)

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, kubernetes.CreateServiceMonitor(instance.Namespace, actions.ApiComponentName),
		ensure.ControllerReference[*unstructured.Unstructured](instance, i.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),

		ensure.Optional(statusTLS(instance).CertRef != nil,
			kubernetes.EnsureServiceMonitorSpec(
				labels.ForComponent(actions.ApiComponentName, instance.Name),
				kubernetes.ServiceMonitorHttpsEndpoint(
					actions.ApiMetricsPortName,
					fmt.Sprintf("%s.%s.svc", actions.ApiDeploymentName, instance.Namespace),
					statusTLS(instance).CertRef,
				),
			)),
		ensure.Optional(statusTLS(instance).CertRef == nil,
			kubernetes.EnsureServiceMonitorSpec(
				labels.ForComponent(actions.ApiComponentName, instance.Name),
				kubernetes.ServiceMonitorEndpoint(actions.ApiMetricsPortName),
			)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance,
			metav1.Condition{
				Type:    actions.ApiCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
