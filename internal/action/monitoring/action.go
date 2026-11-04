package monitoring

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/servicemonitor"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
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
	objectMeta := metav1ac.ObjectMeta().WithName(a.serviceMonitorName).WithNamespace(instance.GetNamespace())
	if !a.cfg.IsEnabled(instance) {
		sm, err := servicemonitor.CreateServiceMonitor(objectMeta)
		if err != nil {
			return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorDelete, err), instance)
		}
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
	if _, err := traits.Apply(objectMeta,
		traits.ControllerReference[*metav1ac.ObjectMetaApplyConfiguration](instance, a.Client),
		traits.Labels[*metav1ac.ObjectMetaApplyConfiguration](monitoringLabels),
	); err != nil {
		return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorCreate, err), instance)
	}

	sm, err := servicemonitor.CreateServiceMonitor(objectMeta)
	if err != nil {
		return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorCreate, err), instance)
	}

	endpoint := servicemonitor.ServiceMonitorEndpoint("metrics")
	if tls := a.cfg.TLS(instance); tls.CertRef != nil {
		serverName := fmt.Sprintf("%s.%s.svc", a.serviceMonitorName, instance.GetNamespace())
		endpoint = servicemonitor.ServiceMonitorHttpsEndpoint("metrics", serverName, tls.CertRef)
	}
	if _, err := traits.Apply(sm, servicemonitor.EnsureServiceMonitorSpec(selectorLabels, endpoint)); err != nil {
		return a.handleError(ctx, fmt.Errorf("%w: %w", ErrServiceMonitorCreate, err), instance)
	}

	if _, err := kubernetes.Apply(ctx, a.Client, client.ApplyConfigurationFromUnstructured(sm), sm.DeepCopy()); err != nil {
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
