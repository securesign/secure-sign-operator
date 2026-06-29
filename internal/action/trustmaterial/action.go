package trustmaterial

import (
	"context"
	"fmt"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewAction[T apis.ConditionsAwareObject](resolver Resolver[T]) action.Action[T] {
	return &resolveAction[T]{resolver: resolver}
}

type resolveAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	resolver Resolver[T]
}

func (a *resolveAction[T]) Name() string {
	return fmt.Sprintf("resolve %s trust material", a.resolver.ComponentName())
}

func (a *resolveAction[T]) CanHandle(ctx context.Context, instance T) bool {
	return a.resolver.CanHandle(ctx, instance)
}

func (a *resolveAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	resolved, err := a.resolver.Resolve(ctx, a.Client, instance)
	if err != nil {
		a.Logger.V(1).Info("failed to resolve trust material, will retry", "error", err)
		instance.SetCondition(metav1.Condition{
			Type:    a.resolver.ConditionType(),
			Status:  metav1.ConditionFalse,
			Reason:  state.Initialize.String(),
			Message: fmt.Sprintf("Resolving %s trust material: %v", a.resolver.ComponentName(), err),
		})
		if _, err := a.PersistStatus(ctx, instance); err != nil {
			return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), err), instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	if err = ValidatePEM(resolved); err != nil {
		a.Logger.V(1).Info("resolved data is not valid PEM, will retry", "error", err)
		instance.SetCondition(metav1.Condition{
			Type:    a.resolver.ConditionType(),
			Status:  metav1.ConditionFalse,
			Reason:  state.Initialize.String(),
			Message: fmt.Sprintf("Resolving %s trust material: invalid PEM response", a.resolver.ComponentName()),
		})
		if _, err := a.PersistStatus(ctx, instance); err != nil {
			return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), err), instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	pem := string(resolved)
	current := a.resolver.GetTrustMaterial(instance)
	if current == pem {
		return a.Continue()
	}

	a.resolver.SetTrustMaterial(instance, pem)

	if _, err = a.PersistStatus(ctx, instance); err != nil {
		return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), err), instance)
	}

	if current == "" {
		a.Recorder.Eventf(instance, nil, "Normal", "TrustMaterialResolved",
			"Resolved", "Resolved %s trust material from running service", a.resolver.ComponentName())
	} else {
		a.Recorder.Eventf(instance, nil, "Normal", "TrustMaterialUpdated",
			"Updated", "Updated %s trust material — service key/cert has changed", a.resolver.ComponentName())
	}

	return a.Continue()
}
