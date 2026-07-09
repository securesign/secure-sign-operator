package monitoring

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewAction creates a generic monitoring action that manages a Prometheus
// ServiceMonitor for a component.
func NewAction[T apis.ConditionsAwareObject](
	componentName string,
	monitoringRoleName string,
	serviceMonitorName string,
	conditionType string,
	cfg Config[T],
) action.Action[T] {
	return &monitoringAction[T]{
		componentName:      componentName,
		monitoringRoleName: monitoringRoleName,
		serviceMonitorName: serviceMonitorName,
		conditionType:      conditionType,
		cfg:                cfg,
	}
}

type monitoringAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	componentName      string
	monitoringRoleName string
	serviceMonitorName string
	conditionType      string
	cfg                Config[T]
}

func (a *monitoringAction[T]) Name() string {
	return "create monitoring"
}

func (a *monitoringAction[T]) CanHandle(_ context.Context, instance T) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (a *monitoringAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	sm := kubernetes.CreateServiceMonitor(instance.GetNamespace(), a.serviceMonitorName)

	if !a.cfg.IsEnabled(instance) {
		if err := a.Client.Delete(ctx, sm); err != nil {
			if client.IgnoreNotFound(err) == nil || meta.IsNoMatchError(err) {
				return a.Continue()
			}
			return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorDelete, err), instance)
		}
		return a.Continue()
	}

	monitoringLabels := labels.For(a.componentName, a.monitoringRoleName, instance.GetName())
	selectorLabels := labels.ForComponent(a.componentName, instance.GetName())

	ensureFns := []func(*unstructured.Unstructured) error{
		ensure.ControllerReference[*unstructured.Unstructured](instance, a.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
	}

	if tls := a.cfg.TLS(instance); tls.CertRef != nil {
		serverName := fmt.Sprintf("%s.%s.svc", a.serviceMonitorName, instance.GetNamespace())
		ensureFns = append(ensureFns,
			kubernetes.EnsureServiceMonitorSpec(
				selectorLabels,
				kubernetes.ServiceMonitorHttpsEndpoint("metrics", serverName, tls.CertRef),
			),
		)
	} else {
		ensureFns = append(ensureFns,
			kubernetes.EnsureServiceMonitorSpec(
				selectorLabels,
				kubernetes.ServiceMonitorEndpoint("metrics"),
			),
		)
	}

	if _, err := kubernetes.CreateOrUpdate(ctx, a.Client, sm, ensureFns...); err != nil {
		if meta.IsNoMatchError(err) {
			return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorCRDMissing, err), instance)
		}
		return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorCreate, err), instance)
	}

	return a.Continue()
}

func (a *monitoringAction[T]) handleError(ctx context.Context, err error, instance T) *action.Result {
	if a.conditionType != "" {
		return a.Error(ctx, err, instance, metav1.Condition{
			Type:    a.conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}
	return a.Error(ctx, err, instance)
}
