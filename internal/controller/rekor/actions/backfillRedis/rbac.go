package backfillredis

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"k8s.io/apimachinery/pkg/api/meta"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Rekor] {
	return rbac.NewAction[*rhtasv1alpha1.Rekor](actions.BackfillRedisCronJobName, actions.RBACBackfillName,
		rbac.WithCanHandle[*rhtasv1alpha1.Rekor](func(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
			c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
			if c == nil {
				return false
			}
			return (c.Reason == constants.Ready || c.Reason == constants.Creating) && enabled(instance)
		}))
}
