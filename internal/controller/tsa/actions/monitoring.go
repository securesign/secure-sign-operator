package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewMonitoringAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && instance.Spec.Monitoring.Enabled
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	var (
		err error
	)
	monitoringLabels := labels.For(ComponentName, MonitoringRoleName, instance.Name)
	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MonitoringRoleName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*v1.Role](instance, i.Client),
		ensure.Labels[*v1.Role](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
		kubernetes.EnsureRoleRules(
			v1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create monitoring Role: %w", err)), instance)
	}

	// RoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &v1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MonitoringRoleName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*v1.RoleBinding](instance, i.Client),
		ensure.Labels[*v1.RoleBinding](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
		kubernetes.EnsureRoleBinding(
			v1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     MonitoringRoleName,
			},
			v1.Subject{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
		),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create monitoring RoleBinding: %w", err)), instance)
	}

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
