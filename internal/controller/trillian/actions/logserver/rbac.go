package logserver

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/trillian/actions"
)

func NewRBACAction() action.Action[*rhtasv1.Trillian] {
	return rbac.NewAction[*rhtasv1.Trillian](actions.LogServerComponentName, actions.RBACServerName)
}
