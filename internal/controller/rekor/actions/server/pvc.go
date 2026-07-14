package server

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/pvc"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

const PvcNameFormat = "rekor-%s-pvc"

func NewCreatePvcAction() action.Action[*rhtasv1.Rekor] {
	wrapper := pvc.Wrapper[*rhtasv1.Rekor](
		func(r *rhtasv1.Rekor) rhtasv1.Pvc {
			return r.Spec.Attestations.Pvc
		},
		func(r *rhtasv1.Rekor) string {
			return r.Status.PvcName
		},
		func(r *rhtasv1.Rekor, s string) {
			r.Status.PvcName = s
		},
		func(r *rhtasv1.Rekor) bool {
			return enabledFileAttestationStorage(r)
		},
	)

	return pvc.NewAction[*rhtasv1.Rekor](
		PvcNameFormat,
		actions.ServerComponentName,
		actions.ServerDeploymentName,
		wrapper,
	)
}
