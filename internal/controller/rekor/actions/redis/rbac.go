package redis

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Rekor] {
	return rbac.NewAction[*rhtasv1alpha1.Rekor](actions.RedisDeploymentName, actions.RBACRedisName)
}
