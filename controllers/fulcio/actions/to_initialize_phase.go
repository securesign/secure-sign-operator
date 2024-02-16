package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToInitializeAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &toInitialize{}
}

type toInitialize struct {
	action.BaseAction
}

func (i toInitialize) Name() string {
	return "to initialize"
}

func (i toInitialize) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating
}

func (i toInitialize) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Initialize})

	return i.StatusUpdate(ctx, instance)
}
