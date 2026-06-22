package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.Fulcio] {
	return rbac.NewAction[*rhtasv1.Fulcio](ComponentName, RBACName,
		rbac.WithImagePullSecrets(func(instance *rhtasv1.Fulcio) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
