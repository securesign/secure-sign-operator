package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type toPending struct {
	action.BaseAction
}

func NewToPendingPhaseAction() action.Action[rhtasv1alpha1.TimestampAuthority] {
	return &toPending{}
}

func (i toPending) Name() string {
	return "move to pending phase"
}

func (i toPending) CanHandle(_ context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) == nil
}

func (i toPending) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions,
		metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending})

	return i.StatusUpdate(ctx, instance)
}
