package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeStatusAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &initializeStatus{}
}

type initializeStatus struct {
	action.BaseAction
}

func (i initializeStatus) Name() string {
	return "initialize status"
}

func (i initializeStatus) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, constants.Ready) == nil
}

func (i initializeStatus) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	for _, conditionType := range []string{constants.Ready, TrillianCondition, FulcioCondition, RekorCondition, CTlogCondition, TufCondition} {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   conditionType,
			Status: v1.ConditionUnknown,
			Reason: constants.Pending,
		})
	}
	return i.StatusUpdate(ctx, instance)
}
