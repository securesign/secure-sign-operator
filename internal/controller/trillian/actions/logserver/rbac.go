package logserver

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/trillian/actions"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Trillian] {
	return rbac.NewAction[*rhtasv1alpha1.Trillian](actions.LogServerComponentName, actions.RBACServerName)
}
