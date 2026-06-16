package monitor

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.Rekor] {
	return rbac.NewAction[*rhtasv1.Rekor](actions.MonitorComponentName, actions.RBACMonitorName,
		rbac.WithImagePullSecrets(func(instance *rhtasv1.Rekor) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
