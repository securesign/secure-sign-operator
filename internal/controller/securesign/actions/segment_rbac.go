package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	return true
}

func (i rbacAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	result := i.cleanupResource(ctx, instance, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentRBACName,
			Namespace: instance.Namespace,
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	result = i.cleanupResource(ctx, instance, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(namespacedNamePattern, instance.Namespace),
			Namespace: OpenshiftMonitoringNS,
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	result = i.cleanupResource(ctx, instance, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(namespacedNamePattern, instance.Namespace),
			Namespace: OpenshiftMonitoringNS,
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	result = i.cleanupResource(ctx, instance, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterMonitoringRoleBinding"),
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	result = i.cleanupResource(ctx, instance, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRole"),
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	result = i.cleanupResource(ctx, instance, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(clusterWideNamePattern, instance.Namespace, "clusterRoleBinding"),
		},
	})
	if !action.IsContinue(result) {
		return result
	}

	return i.Continue()
}

func (i rbacAction) cleanupResource(ctx context.Context, instance *rhtasv1alpha1.Securesign, object client.Object) *action.Result {
	if err := client.IgnoreNotFound(i.Client.Delete(ctx, object)); err != nil {
		return i.Error(ctx, err, instance,
			metav1.Condition{
				Type:    MetricsCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}
	return i.Continue()
}
