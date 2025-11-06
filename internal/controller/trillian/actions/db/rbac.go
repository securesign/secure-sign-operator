package db

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/rbac"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Trillian] {
	return rbac.NewAction[*rhtasv1alpha1.Trillian](
		actions.DbComponentName, actions.RBACDbName,
		rbac.WithCanHandle[*rhtasv1alpha1.Trillian](func(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
			c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
			if c == nil {
				return false
			}
			return (c.Reason == constants.Ready || c.Reason == constants.Creating) && enabled(instance)
		}),
		rbac.WithImagePullSecrets[*rhtasv1alpha1.Trillian](func(instance *rhtasv1alpha1.Trillian) []v1.LocalObjectReference {
			return instance.Spec.ImagePullSecrets
		}),
	)
}
