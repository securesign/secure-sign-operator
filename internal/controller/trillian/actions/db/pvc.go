package db

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/pvc"
	"github.com/securesign/operator/internal/controller/trillian/actions"
)

func NewCreatePvcAction() action.Action[*rhtasv1alpha1.Trillian] {
	wrapper := pvc.Wrapper[*rhtasv1alpha1.Trillian](
		func(t *rhtasv1alpha1.Trillian) rhtasv1alpha1.Pvc {
			return t.Spec.Db.Pvc
		},
		func(t *rhtasv1alpha1.Trillian) string {
			return t.Status.Db.Pvc.Name
		},
		func(t *rhtasv1alpha1.Trillian, s string) {
			t.Status.Db.Pvc.Name = s
		},
		func(t *rhtasv1alpha1.Trillian) bool {
			return enabled(t)
		},
	)

	return pvc.NewAction[*rhtasv1alpha1.Trillian](
		actions.DbPvcName,
		actions.DbComponentName,
		actions.DbDeploymentName,
		wrapper,
	)
}
