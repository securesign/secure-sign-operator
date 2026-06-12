package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/tree"
)

func NewResolveTreeAction() action.Action[*rhtasv1.CTlog] {
	wrapper := tree.Wrapper[*rhtasv1.CTlog](
		func(rekor *rhtasv1.CTlog) *int64 {
			return rekor.Spec.TreeID
		},
		func(rekor *rhtasv1.CTlog) *int64 {
			return rekor.Status.TreeID
		},
		func(rekor *rhtasv1.CTlog, i *int64) {
			rekor.Status.TreeID = i
		},
		func(rekor *rhtasv1.CTlog) *rhtasv1.TrillianService {
			return &rekor.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1.CTlog]("ctlog", wrapper)
}
