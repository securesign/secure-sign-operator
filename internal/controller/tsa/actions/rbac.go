package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return rbac.NewAction[*rhtasv1alpha1.TimestampAuthority](
		ComponentName, RBACName,
		rbac.WithImagePullSecrets[*rhtasv1alpha1.TimestampAuthority](func(instance *rhtasv1alpha1.TimestampAuthority) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
