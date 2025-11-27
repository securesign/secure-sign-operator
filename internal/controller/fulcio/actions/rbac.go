package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return rbac.NewAction[*rhtasv1alpha1.Fulcio](
		ComponentName, RBACName,
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Fulcio](func(instance *rhtasv1alpha1.Fulcio) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
