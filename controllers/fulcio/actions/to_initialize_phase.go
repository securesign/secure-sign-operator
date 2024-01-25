package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
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

func (i toInitialize) CanHandle(instance *rhtasv1alpha1.Fulcio) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i toInitialize) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	instance.Status.Phase = rhtasv1alpha1.PhaseInitialize

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseInitialize)})

	return i.StatusUpdate(ctx, instance)
}
