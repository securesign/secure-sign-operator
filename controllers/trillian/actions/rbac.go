package actions

import (
	"context"
	"fmt"

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

func NewRBACAction() action.Action[rhtasv1alpha1.Trillian] {
	return &rbacAction{}
}

type rbacAction struct {
	action.BaseAction
}

func (i rbacAction) Name() string {
	return "ensure RBAC"
}

func (i rbacAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i rbacAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err error
	)
	labels := constants.LabelsFor(LogServerComponentName, RBACName, instance.Name)

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RBACName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		ImagePullSecrets: []v1.LocalObjectReference{
			{
				Name: "pull-secret",
			},
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
	role := kubernetes.CreateRole(instance.Namespace, RBACName, labels, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create", "get", "update"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"create", "get", "update"},
		},
	})

	if err = ctrl.SetControllerReference(instance, role, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for role: %w", err))
	}
	_, err = i.Ensure(ctx, role)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Role: %w", err), instance)
	}
	rb := kubernetes.CreateRoleBinding(instance.Namespace, RBACName, labels, rbacv1.RoleRef{
		APIGroup: v1.SchemeGroupVersion.Group,
		Kind:     "Role",
		Name:     RBACName,
	},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: RBACName, Namespace: instance.Namespace},
		})

	if err = ctrl.SetControllerReference(instance, rb, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for roleBinding: %w", err))
	}
	_, err = i.Ensure(ctx, rb)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create RoleBinding: %w", err), instance)
	}
	return i.Continue()
}
