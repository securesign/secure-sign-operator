package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Rekor] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	components := []string{ServerComponentName, RedisComponentName}
	if instance.Spec.RekorSearchUI.Enabled {
		components = append(components, UIComponentName)
	}
	for _, c := range components {
		if !meta.IsStatusConditionTrue(instance.Status.Conditions, c+"Ready") {
			// deployment is watched - no need to requeue
			return i.Return()
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseReady)})
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return i.StatusUpdate(ctx, instance)
}
