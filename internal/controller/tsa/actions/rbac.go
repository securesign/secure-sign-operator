package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.TimestampAuthority] {
	return rbac.NewAction[*rhtasv1.TimestampAuthority](ComponentName, RBACName,
		rbac.WithImagePullSecrets(func(instance *rhtasv1.TimestampAuthority) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
