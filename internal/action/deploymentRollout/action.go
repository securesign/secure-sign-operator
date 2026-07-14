package deploymentRollout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	commonUtils "github.com/securesign/operator/internal/utils/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config parameterizes a rollout-check action for a single component.
type Config[T apis.ConditionsAwareObject] struct {
	// Name is returned by Action.Name(), used for log scoping.
	Name string
	// ConditionType is the status condition this action sets on success/failure (the overall
	// Ready condition, or a component's own sub-condition). CanHandle always gates on the CR's
	// main Ready condition, regardless of ConditionType.
	ConditionType string
	// DeploymentName is the component's fixed Deployment name constant.
	DeploymentName string
	// Enabled optionally gates the whole action (e.g. a spec.enabled toggle). Nil means always enabled.
	Enabled func(T) bool
	// PromoteOnSuccess: true sets ConditionType=True/Ready and persists once the Deployment is rolled
	// out; false just Continue()s, leaving promotion to transitions.NewToReadyPhaseAction.
	PromoteOnSuccess bool
}

// NewAction returns a generic action that keeps ConditionType in sync with the live rollout status
// of the component's Deployment, from the point it's first created onward — including re-verifying
// after the condition has already reached True, so a later regression (image bump, deleted Deployment,
// stuck rollout) is detected instead of being permanently masked.
func NewAction[T apis.ConditionsAwareObject](cfg Config[T]) action.Action[T] {
	return &rolloutCheck[T]{cfg: cfg}
}

type rolloutCheck[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	cfg Config[T]
}

func (a rolloutCheck[T]) Name() string {
	return a.cfg.Name
}

func (a rolloutCheck[T]) CanHandle(_ context.Context, instance T) bool {
	if a.cfg.Enabled != nil && !a.cfg.Enabled(instance) {
		return false
	}
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (a rolloutCheck[T]) Handle(ctx context.Context, instance T) *action.Result {
	ok, err := commonUtils.DeploymentIsRunningByName(ctx, a.Client, instance.GetNamespace(), a.cfg.DeploymentName)
	switch {
	case errors.Is(err, commonUtils.ErrDeploymentNotReady):
		a.Logger.V(1).Info("deployment is not ready", "error", err.Error())
	case err != nil:
		return a.Error(ctx, err, instance)
	}

	if !ok {
		a.Logger.V(1).Info("Waiting for deployment")
		message := "deployment not ready"
		if err != nil {
			message = err.Error()
		}
		instance.SetCondition(metav1.Condition{
			Type:               a.cfg.ConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             state.Initialize.String(),
			Message:            message,
			ObservedGeneration: instance.GetGeneration(),
		})
		// A sub-condition regressing must also be visible on the CR's main Ready condition —
		// otherwise, once Ready, the chain halts here (RequeueAfter) before ever reaching
		// transitions.NewToReadyPhaseAction, and Ready is never demoted from a stale True.
		if a.cfg.ConditionType != constants.ReadyCondition {
			instance.SetCondition(metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Initialize.String(),
				Message:            fmt.Sprintf("Waiting for %s", a.cfg.DeploymentName),
				ObservedGeneration: instance.GetGeneration(),
			})
		}
		if _, err := a.PersistStatus(ctx, instance); err != nil {
			return a.Error(ctx, err, instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	if !a.cfg.PromoteOnSuccess {
		return a.Continue()
	}

	instance.SetCondition(metav1.Condition{
		Type:               a.cfg.ConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		ObservedGeneration: instance.GetGeneration(),
	})
	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}
