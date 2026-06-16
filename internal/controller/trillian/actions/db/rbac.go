package db

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/state"
	v1 "k8s.io/api/core/v1"
)

func NewRBACAction() action.Action[*rhtasv1.Trillian] {
	return rbac.NewAction[*rhtasv1.Trillian](
		actions.DbComponentName, actions.RBACDbName,
		rbac.WithCanHandle[*rhtasv1.Trillian](func(_ context.Context, instance *rhtasv1.Trillian) bool {
			return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
		}),
		rbac.WithImagePullSecrets(func(instance *rhtasv1.Trillian) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
