package ui

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
)

func NewRBACAction() action.Action[*rhtasv1.Rekor] {
	return rbac.NewAction[*rhtasv1.Rekor](
		actions.SearchUiDeploymentName, actions.RBACUIName,
		rbac.WithCanHandle[*rhtasv1.Rekor](func(_ context.Context, instance *rhtasv1.Rekor) bool {
			return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
		}))
}
