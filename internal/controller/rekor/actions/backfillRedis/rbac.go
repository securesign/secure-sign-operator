package backfillredis

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Rekor] {
	return rbac.NewAction[*rhtasv1alpha1.Rekor](actions.BackfillRedisCronJobName, actions.RBACBackfillName,
		rbac.WithCanHandle[*rhtasv1alpha1.Rekor](func(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
			return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
		}))
}
