package actions

import (
	"context"
	"sort"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewUpdateStatusAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &updateStatusAction{}
}

type updateStatusAction struct {
	action.BaseAction
}

func (i updateStatusAction) Name() string {
	return "update status"
}

func (i updateStatusAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Securesign) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) != nil
}

func (i updateStatusAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	sorted := sortByStatus(instance.Status.Conditions)

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, sorted[0]) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   constants.Ready,
			Status: v1.ConditionFalse,
			Reason: meta.FindStatusCondition(instance.Status.Conditions, sorted[0]).Reason,
		})
		return i.StatusUpdate(ctx, instance)
	}
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, constants.Ready) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   constants.Ready,
			Status: v1.ConditionTrue,
			Reason: constants.Ready,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}

func sortByStatus(conditions []v1.Condition) []string {
	sorted := []string{TrillianCondition, FulcioCondition, RekorCondition, CTlogCondition, TufCondition, TSACondition}
	sort.SliceStable(sorted, func(i, j int) bool {
		iCondition := meta.FindStatusCondition(conditions, sorted[i])
		jCondition := meta.FindStatusCondition(conditions, sorted[j])

		order := map[string]int{
			constants.Pending:    0,
			constants.Initialize: 1,
			constants.Creating:   2,
			constants.Ready:      3,
			constants.NotDefined: 4,
		}

		return order[iCondition.Reason] < order[jCondition.Reason]
	})
	return sorted
}
