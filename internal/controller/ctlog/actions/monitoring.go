package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewCreateMonitorAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	return instance.Spec.Monitoring.Enabled && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err error
	)

	monitoringLabels := labels.For(ComponentName, MonitoringRoleName, instance.Name)

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, kubernetes.CreateServiceMonitor(instance.Namespace, DeploymentName),
		ensure.ControllerReference[*unstructured.Unstructured](instance, i.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
		kubernetes.EnsureServiceMonitorSpec(
			labels.ForComponent(ComponentName, instance.Name),
			kubernetes.ServiceMonitorEndpoint(MetricsPortName),
		),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance)
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
