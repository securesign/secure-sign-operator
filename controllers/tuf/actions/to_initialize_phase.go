package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToInitializePhaseAction() action.Action[rhtasv1alpha1.Tuf] {
	return &toInitialize{}
}

type toInitialize struct {
	action.BaseAction
}

func (i toInitialize) Name() string {
	return "move to initialize"
}

func (i toInitialize) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Status == metav1.ConditionFalse && (c.Reason == constants.Creating)
}

func (i toInitialize) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Initialize, Message: "Move to initialize phase"})

	return i.StatusUpdate(ctx, instance)
}
