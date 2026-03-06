package db

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/pvc"
	"github.com/securesign/operator/internal/controller/console/actions"
)

func NewCreatePvcAction() action.Action[*rhtasv1alpha1.Console] {
	wrapper := pvc.Wrapper[*rhtasv1alpha1.Console](
		func(t *rhtasv1alpha1.Console) rhtasv1alpha1.Pvc {
			return t.Spec.Db.Pvc
		},
		func(t *rhtasv1alpha1.Console) string {
			return t.Status.Db.Pvc.Name
		},
		func(t *rhtasv1alpha1.Console, s string) {
			t.Status.Db.Pvc.Name = s
		},
		func(t *rhtasv1alpha1.Console) bool {
			return enabled(t)
		},
	)

	return pvc.NewAction[*rhtasv1alpha1.Console](
		actions.DbPvcName,
		actions.DbComponentName,
		actions.DbDeploymentName,
		wrapper,
	)
}
