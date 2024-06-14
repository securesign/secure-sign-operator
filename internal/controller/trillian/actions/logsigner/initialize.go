package logsigner

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	commonUtils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "server initialize"
}

func (i initializeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Initialize && !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	labels := constants.LabelsForComponent(actions.LogSignerComponentName, instance.Name)
	ok, err := commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Error(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Initialize,
			Message: "Waiting for deployment to be ready",
		})
		return i.StatusUpdate(ctx, instance)
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.SignerCondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready})
	return i.StatusUpdate(ctx, instance)
}

func (i initializeAction) CanHandleError(_ context.Context, _ *rhtasv1alpha1.Trillian) bool {
	return false
}

func (i initializeAction) HandleError(_ context.Context, _ *rhtasv1alpha1.Trillian) *action.Result {
	return i.Continue()
}
