package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/pvc"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
)

func NewCreatePvcAction() action.Action[*rhtasv1.Tuf] {
	wrapper := pvc.Wrapper[*rhtasv1.Tuf](
		func(t *rhtasv1.Tuf) rhtasv1.Pvc {
			return t.Spec.Pvc
		},
		func(t *rhtasv1.Tuf) string {
			return t.Status.PvcName
		},
		func(t *rhtasv1.Tuf, s string) {
			t.Status.PvcName = s
		},
		func(t *rhtasv1.Tuf) bool {
			return true
		},
	)

	return pvc.NewAction[*rhtasv1.Tuf](
		"tuf",
		tufConstants.DeploymentName,
		tufConstants.DeploymentName,
		wrapper,
	)
}
