package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.Rekor] {
	return rbac.NewAction[*rhtasv1.Rekor](actions.RedisDeploymentName, actions.RBACRedisName,
		rbac.WithCanHandle(func(ctx context.Context, instance *rhtasv1.Rekor) bool {
			return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
		}),
		rbac.WithImagePullSecrets(func(instance *rhtasv1.Rekor) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
