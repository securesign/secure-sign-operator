package monitoring

import (
	"context"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MonitoringConfig holds configuration for creating a monitoring action
type MonitoringConfig[T apis.ConditionsAwareObject] struct {
	ComponentName         string
	DeploymentName        string
	MonitoringRoleName    string
	MetricsPortName       string
	IsMonitoringEnabled   func(T) bool
	CustomEndpointBuilder func(instance T) []func(*unstructured.Unstructured) error
	Registry              *ServiceMonitorRegistry
}

// NewMonitoringAction creates a generic monitoring action for any supported instance type
func NewMonitoringAction[T apis.ConditionsAwareObject](config MonitoringConfig[T]) action.Action[T] {
	return &genericMonitoringAction[T]{
		config: config,
	}
}

type genericMonitoringAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	config MonitoringConfig[T]
}

func (i *genericMonitoringAction[T]) Name() string {
	return "create monitoring"
}

func (i *genericMonitoringAction[T]) CanHandle(ctx context.Context, instance T) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && i.config.IsMonitoringEnabled(instance)
}

func (i *genericMonitoringAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	registry := i.config.Registry
	key := client.ObjectKeyFromObject(instance)

	monitoringLabels := labels.For(i.config.ComponentName, i.config.MonitoringRoleName, instance.GetName())

	ensureFuncs := []func(*unstructured.Unstructured) error{
		ensure.ControllerReference[*unstructured.Unstructured](instance, i.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
	}

	if i.config.CustomEndpointBuilder != nil {
		ensureFuncs = append(ensureFuncs, i.config.CustomEndpointBuilder(instance)...)
	} else {
		ensureFuncs = append(ensureFuncs, kubernetes.EnsureServiceMonitorSpec(
			labels.ForComponent(i.config.ComponentName, instance.GetName()),
			kubernetes.ServiceMonitorEndpoint(i.config.MetricsPortName),
		))
	}

	spec := &ServiceMonitorSpec{
		OwnerKey:    key,
		OwnerGVK:    instance.GetObjectKind().GroupVersionKind(),
		Namespace:   instance.GetNamespace(),
		Name:        i.config.DeploymentName,
		EnsureFuncs: ensureFuncs,
	}

	registry.Register(ctx, spec, instance)

	if err := registry.ReconcileOne(ctx, spec); err != nil {
		i.Logger.V(1).Info("ServiceMonitor reconciliation deferred", "error", err.Error())
	} else {
		i.Logger.V(1).Info("ServiceMonitor reconciled successfully", "name", spec.Name)
	}

	return i.Continue()
}
