package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToCreatePhaseAction() action.Action[rhtasv1alpha1.Trillian] {
	return &toCreate{}
}

type toCreate struct {
	action.BaseAction
}

func (i toCreate) Name() string {
	return "move to create phase"
}

func (i toCreate) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason == constants.Pending
}

func (i toCreate) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating})
	return i.StatusUpdate(ctx, instance)
}
