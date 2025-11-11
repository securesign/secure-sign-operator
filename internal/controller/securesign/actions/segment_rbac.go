package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	namespacedNamePattern  = SegmentRBACName + "-%s"
	clusterWideNamePattern = SegmentRBACName + "-%s" + "-%s"
	OpenshiftMonitoringNS  = "openshift-monitoring"
)

func NewSBJRBACAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &rbacAction{}
}

type rbacAction struct {
	action.BaseAction
}

func (i rbacAction) Name() string {
	return "ensure RBAC for segment job"
}

func (i rbacAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	if !kubernetes.IsOpenShift() {
		return false
	}

	c := meta.FindStatusCondition(instance.Status.Conditions, MetricsCondition)
	if c == nil || c.Reason == constants.Ready {
		return false
	}
	val, found := instance.Annotations[annotations.Metrics]
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

	jobLabels := labels.For(SegmentBackupJobName, SegmentBackupCronJobName, instance.Name)
	jobLabels[labels.LabelAppNamespace] = instance.Namespace

	// ServiceAccount
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentRBACName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*v1.ServiceAccount](instance, i.Client),
		ensure.Labels[*v1.ServiceAccount](slices.Collect(maps.Keys(jobLabels)), jobLabels),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create SA: %w", err)), instance)
	}

	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(namespacedNamePattern, instance.Namespace),
			Namespace: OpenshiftMonitoringNS,
		},
	},
		ensure.Labels[*rbacv1.Role](slices.Collect(maps.Keys(jobLabels)), jobLabels),
		kubernetes.EnsureRoleRules(
			rbacv1.PolicyRule{

				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"cluster-monitoring-config"},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{"route.openshift.io"},
				Resources: []string{"routes"},
				Verbs:     []string{"get", "list"},
			},
		),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create openshift-monitoring role for SBJ: %w", err)), instance)
	}

	// RoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(namespacedNamePattern, instance.Namespace),
			Namespace: OpenshiftMonitoringNS,
		},
	},
		ensure.Labels[*rbacv1.RoleBinding](slices.Collect(maps.Keys(jobLabels)), jobLabels),
		kubernetes.EnsureRoleBinding(
			rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     fmt.Sprintf(namespacedNamePattern, instance.Namespace),
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create openshift-monitoring role binding for SBJ: %w", err)), instance)
	}

	// ClusterRoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterMonitoringRoleBinding"),
		},
	},
		ensure.Labels[*rbacv1.ClusterRoleBinding](slices.Collect(maps.Keys(jobLabels)), jobLabels),
		kubernetes.EnsureClusterRoleBinding(
			rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "ClusterRole",
				Name:     "cluster-monitoring-view",
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create monitoring ClusterRoleBinding for SBJ: %w", err)), instance)
	}

	// ClusterRole
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRole"),
		},
	},
		ensure.Labels[*rbacv1.ClusterRole](slices.Collect(maps.Keys(jobLabels)), jobLabels),
		kubernetes.EnsureClusterRoleRules(
			rbacv1.PolicyRule{
				APIGroups:     []string{"operator.openshift.io"},
				Resources:     []string{"consoles"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"cluster"},
			},
			rbacv1.PolicyRule{
				APIGroups:     []string{"route.openshift.io"},
				Resources:     []string{"routes"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{"console"},
			},
		),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create openshift-console ClusterRole for SBJ: %w", err)), instance)
	}

	// ClusterRoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRoleBinding"),
		},
	},
		ensure.Labels[*rbacv1.ClusterRoleBinding](slices.Collect(maps.Keys(jobLabels)), jobLabels),
		kubernetes.EnsureClusterRoleBinding(
			rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "ClusterRole",
				Name:     fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRole"),
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		),
	); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create openshift-console ClusterRoleBinding for SBJ: %w", err)), instance)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    MetricsCondition,
		Status:  metav1.ConditionTrue,
		Reason:  constants.Creating,
		Message: "Segment Backup Job Creating",
	})

	return i.Continue()
}
