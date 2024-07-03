package logsigner

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	monitoringLabels := constants.LabelsFor(actions.LogSignerComponentName, actions.LogSignerMonitoringName, instance.Name)
	role := kubernetes.CreateRole(
		instance.Namespace,
		actions.LogSignerMonitoringName,
		monitoringLabels,
		[]v1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	)

	if err = controllerutil.SetControllerReference(instance, role, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for role: %w", err))
	}

	if _, err = i.Ensure(ctx, role); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create monitoring role: %w", err), instance)
	}

	roleBinding := kubernetes.CreateRoleBinding(
		instance.Namespace,
		actions.LogSignerMonitoringName,
		monitoringLabels,
		v1.RoleRef{
			APIGroup: v1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     actions.LogSignerMonitoringName,
		},
		[]v1.Subject{
			{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
		},
	)
	if err = controllerutil.SetControllerReference(instance, roleBinding, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for role: %w", err))
	}

	if _, err = i.Ensure(ctx, roleBinding); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create monitoring RoleBinding: %w", err), instance)
	}

	serviceMonitor := kubernetes.CreateServiceMonitor(
		instance.Namespace,
		actions.LogSignerComponentName,
		monitoringLabels,
		[]monitoringv1.Endpoint{
			{
				Interval: monitoringv1.Duration("30s"),
				Port:     actions.LogSignerComponentName,
				Scheme:   "http",
			},
		},
		constants.LabelsForComponent(actions.LogSignerComponentName, instance.Name),
	)

	if err = controllerutil.SetControllerReference(instance, serviceMonitor, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for serviceMonitor: %w", err))
	}

	if _, err = i.Ensure(ctx, serviceMonitor); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance)
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
