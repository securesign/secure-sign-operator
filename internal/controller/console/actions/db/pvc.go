package db

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/pvc"
	"github.com/securesign/operator/internal/controller/console/actions"
)

func NewCreatePvcAction() action.Action[*rhtasv1.Console] {
	wrapper := pvc.Wrapper[*rhtasv1.Console](
		func(t *rhtasv1.Console) rhtasv1.Pvc {
			return t.Spec.Db.Pvc
		},
		func(t *rhtasv1.Console) string {
			return t.Status.Db.Pvc.Name
		},
		func(t *rhtasv1.Console, s string) {
			t.Status.Db.Pvc.Name = s
		},
		func(t *rhtasv1.Console) bool {
			return enabled(t)
		},
	)

	return pvc.NewAction[*rhtasv1.Console](
		actions.DbPvcName,
		actions.DbComponentName,
		actions.DbDeploymentName,
		wrapper,
	)
}
