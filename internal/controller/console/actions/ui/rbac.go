package ui

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/console/actions"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Console] {
	return rbac.NewAction[*rhtasv1alpha1.Console](actions.UIComponentName, actions.RBACUIName)
}
