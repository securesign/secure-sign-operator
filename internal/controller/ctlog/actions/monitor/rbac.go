package monitor

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.CTlog] {
	return rbac.NewAction[*rhtasv1alpha1.CTlog](actions.MonitorComponentName, actions.RBACMonitorName)
}
