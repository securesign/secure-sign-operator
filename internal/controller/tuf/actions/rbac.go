package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Tuf] {
	return rbac.NewAction[*rhtasv1alpha1.Tuf](
		tufConstants.ComponentName, tufConstants.RBACName,
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Tuf](func(instance *rhtasv1alpha1.Tuf) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}

func NewRBACInitJobAction() action.Action[*rhtasv1alpha1.Tuf] {
	return rbac.NewAction[*rhtasv1alpha1.Tuf](
		tufConstants.ComponentName, tufConstants.RBACInitJobName,
		rbac.WithRule[*rhtasv1alpha1.Tuf](
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "update"},
			}),
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Tuf](func(instance *rhtasv1alpha1.Tuf) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
