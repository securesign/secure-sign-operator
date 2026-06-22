package logserver

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.Trillian] {
	return rbac.NewAction[*rhtasv1.Trillian](actions.LogServerComponentName, actions.RBACServerName,
		rbac.WithImagePullSecrets(func(instance *rhtasv1.Trillian) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
