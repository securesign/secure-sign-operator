package actions

import (
	"context"
	"errors"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	commonUtils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[*rhtasv1.Rekor] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return meta.IsStatusConditionFalse(instance.Status.Conditions, actions.RedisCondition) && enabled(instance)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := labels.ForComponent(actions.RedisComponentName, instance.Name)
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
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Initialize.String(),
			Message: "Waiting for deployment to be ready",
		})
		if _, err := i.PersistStatus(ctx, instance); err != nil {
			return i.Error(ctx, err, instance)
		}
		return i.RequeueAfter(5 * time.Second)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.RedisCondition,
		Status: metav1.ConditionTrue, Reason: state.Ready.String()})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
