package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/tree"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
)

func NewResolveTreeAction() action.Action[*rhtasv1.CTlog] {
	wrapper := tree.Wrapper[*rhtasv1.CTlog](
		func(ctlog *rhtasv1.CTlog) *int64 {
			if active := utils.ActiveLog(ctlog.Spec.Logs); active != nil {
				return active.LogId
			}
			return nil
		},
		func(ctlog *rhtasv1.CTlog) *int64 {
			return ctlog.Status.TreeID
		},
		func(ctlog *rhtasv1.CTlog, i *int64) {
			ctlog.Status.TreeID = i
		},
		func(ctlog *rhtasv1.CTlog) *rhtasv1.ServiceReference {
			return &ctlog.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1.CTlog]("ctlog", wrapper)
}
