package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var conditions = []string{
	constants.Ready, TrillianCondition, FulcioCondition, RekorCondition, CTlogCondition, TufCondition, TSACondition, MetricsCondition,
}

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
	for _, condition := range conditions {
		if c := meta.FindStatusCondition(instance.Status.Conditions, condition); c == nil {
			return true
		}
	}
	return false
}

func (i initializeStatus) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	for _, conditionType := range conditions {
		if c := meta.FindStatusCondition(instance.Status.Conditions, conditionType); c == nil {
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   conditionType,
				Status: v1.ConditionUnknown,
				Reason: constants.Pending,
			})
		}
	}
	return i.StatusUpdate(ctx, instance)
}
