package logsigner

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Trillian] {
	return rbac.NewAction[*rhtasv1alpha1.Trillian](
		actions.LogSignerComponentName, actions.RBACSignerName,
		rbac.WithRule[*rhtasv1alpha1.Trillian](
			rbacv1.PolicyRule{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"create", "get", "update", "watch", "patch"},
			}),
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Trillian](func(instance *rhtasv1alpha1.Trillian) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
