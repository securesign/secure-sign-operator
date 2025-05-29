package logserver

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewCreateMonitorAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && instance.Spec.Monitoring.Enabled
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err error
	)

	monitoringLabels := labels.For(actions.LogServerComponentName, actions.LogServerMonitoringName, instance.Name)
	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      actions.LogServerMonitoringName,
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
			Name:      actions.LogServerMonitoringName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*v1.RoleBinding](instance, i.Client),
		ensure.Labels[*v1.RoleBinding](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),
		kubernetes.EnsureRoleBinding(
			v1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     actions.LogServerMonitoringName,
			},
			v1.Subject{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
		),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create monitoring RoleBinding: %w", err)), instance)
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, kubernetes.CreateServiceMonitor(instance.Namespace, actions.LogServerComponentName),
		ensure.ControllerReference[*unstructured.Unstructured](instance, i.Client),
		ensure.Labels[*unstructured.Unstructured](slices.Collect(maps.Keys(monitoringLabels)), monitoringLabels),

		ensure.Optional(statusTLS(instance).CertRef != nil,
			kubernetes.EnsureServiceMonitorSpec(
				labels.ForComponent(actions.LogServerComponentName, instance.Name),
				kubernetes.ServiceMonitorHttpsEndpoint(
					actions.MetricsPortName,
					fmt.Sprintf("%s.%s.svc", actions.LogserverDeploymentName, instance.Namespace),
					statusTLS(instance).CertRef,
				),
			)),
		ensure.Optional(statusTLS(instance).CertRef == nil,
			kubernetes.EnsureServiceMonitorSpec(
				labels.ForComponent(actions.LogServerComponentName, instance.Name),
				kubernetes.ServiceMonitorEndpoint(actions.MetricsPortName),
			)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance,
			metav1.Condition{
				Type:    actions.ServerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
