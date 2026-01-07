package server

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Rekor] {
	return rbac.NewAction[*rhtasv1alpha1.Rekor](
		actions.ServerDeploymentName, actions.RBACName,
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Rekor](func(instance *rhtasv1alpha1.Rekor) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
