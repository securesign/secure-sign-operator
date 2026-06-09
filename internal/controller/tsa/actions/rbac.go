package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
)

func NewRBACAction() action.Action[*rhtasv1.TimestampAuthority] {
	return rbac.NewAction[*rhtasv1.TimestampAuthority](ComponentName, RBACName)
}
