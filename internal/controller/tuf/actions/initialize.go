package actions

import (
	"context"
	"errors"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	commonUtils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Initialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := labels.ForComponent(tufConstants.ComponentName, instance.Name)
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	switch {
	case errors.Is(err, commonUtils.ErrDeploymentNotReady):
		i.Logger.Info("deployment is not ready", "error", err.Error())
	case err != nil:
		return i.Error(ctx, err, instance)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Initialize,
			Message: "Waiting for deployment to be ready",
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
