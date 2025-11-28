package server

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewCreateMonitorAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) &&
		instance.Spec.Monitoring.Enabled &&
		instance.Spec.Monitoring.IsServiceMonitorEnabled(kubernetes.IsOpenShift())
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)

	monitoringLabels := labels.For(actions.ServerComponentName, actions.MonitoringRoleName, instance.Name)

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, kubernetes.CreateServiceMonitor(instance.Namespace, actions.ServerDeploymentName),
		ensure.ControllerReference[*unstructured.Unstructured](instance, i.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
		kubernetes.EnsureServiceMonitorSpec(
			labels.ForComponent(actions.ServerComponentName, instance.Name),
			kubernetes.ServiceMonitorEndpoint(actions.MetricsPortName),
		),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
