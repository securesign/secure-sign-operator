package api

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/console/actions"
)

func NewRBACAction() action.Action[*rhtasv1.Console] {
	return rbac.NewAction[*rhtasv1.Console](actions.ApiComponentName, actions.RBACApiName)
}
