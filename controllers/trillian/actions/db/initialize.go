package db

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Trillian] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "db initialize"
}

func (i initializeAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Initialize && instance.Spec.Db.Create && !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.DbCondition)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	labels := constants.LabelsForComponent(actions.DbComponentName, instance.Name)
	ok, err := commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Initialize,
			Message: "Waiting for deployment to be ready",
		})
		return i.StatusUpdate(ctx, instance)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.DbCondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready, Message: "Db ready"})
	return i.StatusUpdate(ctx, instance)
}
