package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return rbac.NewAction[*rhtasv1alpha1.TimestampAuthority](ComponentName, RBACName)
}
