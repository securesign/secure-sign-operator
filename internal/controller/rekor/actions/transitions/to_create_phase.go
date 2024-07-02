package transitions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToCreateAction() action.Action[rhtasv1alpha1.Rekor] {
	return &toCreateAction{}
}

type toCreateAction struct {
	action.BaseAction
}

func (i toCreateAction) Name() string {
	return "move to create phase"
}

func (i toCreateAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Pending
}

func (i toCreateAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating})

	return i.StatusUpdate(ctx, instance)
}
