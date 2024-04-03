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
	ctrl "sigs.k8s.io/controller-runtime"
)

const namespacedNamePattern = SegmentRBACName + "-%s"

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
	if !instance.Spec.Analytics {
		return i.Continue()
	}
	var err error

	labels := constants.LabelsFor(SegmentBackupCronJobName, SegmentBackupCronJobName, instance.Name)
	labels["app.kubernetes.io/instance-namespace"] = instance.Namespace

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentRBACName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
	}

	if err = ctrl.SetControllerReference(instance, sa, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for SA: %w", err))
	}
	// don't re-enqueue for RBAC in any case (except failure)
	_, err = i.Ensure(ctx, sa)

	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create SA: %w", err), instance)
	}

	role := kubernetes.CreateClusterRole(SegmentRBACName, constants.LabelsRHTAS(), []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"route.openshift.io"},
			Resources: []string{"routes"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"operator.openshift.io"},
			Resources: []string{"consoles"},
			Verbs:     []string{"get", "list"},
		},
	})

	_, err = i.Ensure(ctx, role)

	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create clusterrole required for SBJ: %w", err), instance)
	}

	rb := kubernetes.CreateClusterRoleBinding(fmt.Sprintf(namespacedNamePattern, instance.Namespace), labels, rbacv1.RoleRef{
		APIGroup: v1.SchemeGroupVersion.Group,
		Kind:     "ClusterRole",
		Name:     SegmentRBACName,
	},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		})

	_, err = i.Ensure(ctx, rb)

	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create clusterrolebinding required for SBJ: %w", err), instance)
	}

	return i.Continue()
}
