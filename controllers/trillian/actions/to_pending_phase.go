package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToPendingPhaseAction() action.Action[rhtasv1alpha1.Trillian] {
	return &toPending{}
}

type toPending struct {
	action.BaseAction
}

func (i toPending) Name() string {
	return "move to pending phase"
}

func (i toPending) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) == nil
}

func (i toPending) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Pending})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: DbCondition,
		Status: metav1.ConditionUnknown, Reason: constants.Pending})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: ServerCondition,
		Status: metav1.ConditionUnknown, Reason: constants.Pending})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: SignerCondition,
		Status: metav1.ConditionUnknown, Reason: constants.Pending})
	return i.StatusUpdate(ctx, instance)
}
