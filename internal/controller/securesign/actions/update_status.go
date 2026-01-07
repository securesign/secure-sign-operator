package actions

import (
	"context"
	"sort"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
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
	return meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition) != nil
}

func (i updateStatusAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	sorted := sortByStatus(instance.Status.Conditions)

	if !meta.IsStatusConditionTrue(instance.Status.Conditions, sorted[0]) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:               constants.ReadyCondition,
			Status:             v1.ConditionFalse,
			Reason:             meta.FindStatusCondition(instance.Status.Conditions, sorted[0]).Reason,
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, constants.ReadyCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:               constants.ReadyCondition,
			Status:             v1.ConditionTrue,
			Reason:             state.Ready.String(),
			ObservedGeneration: instance.Generation,
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
			state.Pending.String():    0,
			state.Initialize.String(): 1,
			state.Creating.String():   2,
			state.Ready.String():      3,
			state.NotDefined.String(): 4,
		}

		return order[iCondition.Reason] < order[jCondition.Reason]
	})
	return sorted
}
