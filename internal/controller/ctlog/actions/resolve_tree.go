package actions

import (
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/tree"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
)

func NewResolveTreeAction() action.Action[*rhtasv1.CTlog] {
	wrapper := tree.Wrapper[*rhtasv1.CTlog](
		func(ctlog *rhtasv1.CTlog) *int64 {
			if active := utils.ActiveLog(ctlog.Spec.Logs); active != nil {
				return parseTreeID(active.LogId)
			}
			return nil
		},
		func(ctlog *rhtasv1.CTlog) *int64 {
			return parseTreeID(ctlog.Status.TreeID)
		},
		func(ctlog *rhtasv1.CTlog, i *int64) {
			ctlog.Status.TreeID = formatTreeID(i)
		},
		func(ctlog *rhtasv1.CTlog) *rhtasv1.ServiceReference {
			return &ctlog.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1.CTlog]("ctlog", wrapper)
}

func parseTreeID(s *string) *int64 {
	if s == nil {
		return nil
	}
	v, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func formatTreeID(i *int64) *string {
	if i == nil {
		return nil
	}
	s := strconv.FormatInt(*i, 10)
	return &s
}
