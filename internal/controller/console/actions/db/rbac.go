package db

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
)

func NewRBACAction() action.Action[*rhtasv1.Console] {
	return rbac.NewAction[*rhtasv1.Console](
		actions.DbComponentName, actions.RBACDbName,
		rbac.WithCanHandle[*rhtasv1.Console](func(_ context.Context, instance *rhtasv1.Console) bool {
			c := meta.FindStatusCondition(instance.GetConditions(), state.Ready.String())
			if c == nil {
				return false
			}
			return (c.Reason == state.Ready.String() || c.Reason == state.Creating.String()) && enabled(instance)
		}))
}
