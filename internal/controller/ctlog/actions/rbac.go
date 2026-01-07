package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.CTlog] {
	return rbac.NewAction[*rhtasv1alpha1.CTlog](
		ComponentName, RBACName,
		rbac.WithImagePullSecrets[*rhtasv1alpha1.CTlog](func(instance *rhtasv1alpha1.CTlog) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
