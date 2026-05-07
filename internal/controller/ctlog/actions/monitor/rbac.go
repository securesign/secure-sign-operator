package monitor

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
)

func NewRBACAction() action.Action[*rhtasv1.CTlog] {
	return rbac.NewAction[*rhtasv1.CTlog](actions.MonitorComponentName, actions.RBACMonitorName)
}
