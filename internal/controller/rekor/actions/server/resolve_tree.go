package server

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/tree"
)

func NewResolveTreeAction() action.Action[*rhtasv1.Rekor] {
	wrapper := tree.Wrapper[*rhtasv1.Rekor](
		func(rekor *rhtasv1.Rekor) *int64 {
			return rekor.Spec.TreeID
		},
		func(rekor *rhtasv1.Rekor) *int64 {
			return rekor.Status.TreeID
		},
		func(rekor *rhtasv1.Rekor, i *int64) {
			rekor.Status.TreeID = i
		},
		func(rekor *rhtasv1.Rekor) *rhtasv1.ServiceReference {
			return &rekor.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1.Rekor]("rekor", wrapper)
}
