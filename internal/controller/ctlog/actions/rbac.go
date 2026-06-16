package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.CTlog] {
	return rbac.NewAction[*rhtasv1.CTlog](ComponentName, RBACName,
		rbac.WithImagePullSecrets(func(instance *rhtasv1.CTlog) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
