package actions

import (
	"context"
	"fmt"
	"strconv"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	namespacedNamePattern  = SegmentRBACName + "-%s"
	clusterWideNamePattern = SegmentRBACName + "-%s" + "-%s"
	OpenshiftMonitoringNS  = "openshift-monitoring"
)

func NewRBACAction() action.Action[rhtasv1alpha1.Securesign] {
	return &rbacAction{}
}

type rbacAction struct {
	action.BaseAction
}

func (i rbacAction) Name() string {
	return "ensure RBAC for segment job"
}

func (i rbacAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	val, found := instance.Annotations["rhtas.redhat.com/metrics"]
	if !found {
		return true
	}
	if boolVal, err := strconv.ParseBool(val); err == nil {
		return boolVal
	}
	return true
}

func (i rbacAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var err error

	labels := constants.LabelsFor(SegmentBackupJobName, SegmentBackupCronJobName, instance.Name)
	labels["app.kubernetes.io/instance-namespace"] = instance.Namespace

	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentRBACName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
	}
	if err = controllerutil.SetControllerReference(instance, serviceAccount, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for serviceAccount: %w", err))
	}
	if _, err = i.Ensure(ctx, serviceAccount); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create serviceAccount: %w", err), instance)
	}

	openshiftMonitoringSBJRole := kubernetes.CreateRole(
		OpenshiftMonitoringNS,
		fmt.Sprintf(namespacedNamePattern, instance.Namespace),
		labels,
		[]rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"cluster-monitoring-config"},
			},
			{
				APIGroups: []string{"route.openshift.io"},
				Resources: []string{"routes"},
				Verbs:     []string{"get", "list"},
			},
		})
	if _, err = i.Ensure(ctx, openshiftMonitoringSBJRole); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create openshift-monitoring role for SBJ: %w", err), instance)
	}

	openshiftMonitoringSBJRoleBinding := kubernetes.CreateRoleBinding(
		OpenshiftMonitoringNS,
		fmt.Sprintf(namespacedNamePattern, instance.Namespace),
		labels,
		rbacv1.RoleRef{
			APIGroup: v1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     fmt.Sprintf(namespacedNamePattern, instance.Namespace),
		},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		})
	if _, err = i.Ensure(ctx, openshiftMonitoringSBJRoleBinding); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create openshift-monitoring role binding for SBJ: %w", err), instance)
	}

	openshiftMonitoringClusterRoleBinding := kubernetes.CreateClusterRoleBinding(
		fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterMonitoringRoleBinding"),
		labels,
		rbacv1.RoleRef{
			APIGroup: v1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
		},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		})
	if _, err = i.Ensure(ctx, openshiftMonitoringClusterRoleBinding); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create monitoring ClusterRoleBinding for SBJ: %w", err), instance)
	}

	openshiftConsoleSBJRole := kubernetes.CreateClusterRole(
		fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRole"),
		labels,
		[]rbacv1.PolicyRule{
			{
				APIGroups:     []string{"operator.openshift.io"},
				Resources:     []string{"consoles"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"cluster"},
			},
			{
				APIGroups:     []string{"route.openshift.io"},
				Resources:     []string{"routes"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"console"},
			},
		})
	if _, err = i.Ensure(ctx, openshiftConsoleSBJRole); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create openshift-console ClusterRole for SBJ: %w", err), instance)
	}

	openshiftConsoleSBJRolebinding := kubernetes.CreateClusterRoleBinding(
		fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRoleBinding"),
		labels,
		rbacv1.RoleRef{
			APIGroup: v1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRole"),
		},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		})
	if _, err = i.Ensure(ctx, openshiftConsoleSBJRolebinding); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create openshift-console ClusterRoleBinding for SBJ: %w", err), instance)
	}

	return i.Continue()
}
