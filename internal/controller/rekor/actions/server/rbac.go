package server

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

func NewRBACAction() action.Action[*rhtasv1.Rekor] {
	return rbac.NewAction[*rhtasv1.Rekor](actions.ServerDeploymentName, actions.RBACName)
}
