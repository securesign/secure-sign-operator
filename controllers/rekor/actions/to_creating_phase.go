package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToCreatingAction() action.Action[rhtasv1alpha1.Rekor] {
	return &toCreatingAction{}
}

type toCreatingAction struct {
	action.BaseAction
}

func (i toCreatingAction) Name() string {
	return "move to creating phase"
}

func (i toCreatingAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i toCreatingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	instance.Status.Phase = rhtasv1alpha1.PhaseCreating

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseCreating)})

	return i.StatusUpdate(ctx, instance)
}
