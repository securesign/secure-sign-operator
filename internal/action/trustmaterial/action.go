package trustmaterial

import (
	"context"
	"fmt"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TrustMaterialCondition = "TrustMaterialAvailable"
	ReasonResolveFailed    = "ResolveFailed"
	ReasonResolved         = "Resolved"

	// ReasonDrifted marks a trust material change requiring manual
	// acknowledgement (see [annotations.RefreshTrustMaterial]) before acceptance.
	ReasonDrifted = "Drifted"
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
	cond := meta.FindStatusCondition(instance.GetConditions(), TrustMaterialCondition)
	acknowledged := hasRefreshAcknowledgement(instance)

	resolved, err := a.resolver.Resolve(ctx, a.Client, instance)
	if err != nil {
		return a.handleFetchFailure(ctx, instance, cond,
			fmt.Errorf("resolving %s trust material: %w", a.resolver.ComponentName(), err))
	}

	if err = ValidatePEM(resolved); err != nil {
		return a.handleFetchFailure(ctx, instance, cond,
			fmt.Errorf("resolving %s trust material: invalid PEM response: %w", a.resolver.ComponentName(), err))
	}

	pem := string(resolved)
	current := a.resolver.GetTrustMaterial(instance)
	drifted := current != "" && current != pem

	if drifted && !acknowledged {
		return a.handleDrift(ctx, instance, cond)
	}
	return a.acceptTrustMaterial(ctx, instance, pem, drifted)
}

// handleFetchFailure preserves an existing Drifted marker across transient
// errors instead of clobbering it with ResolveFailed.
func (a *resolveAction[T]) handleFetchFailure(ctx context.Context, instance T, cond *metav1.Condition, err error) *action.Result {
	a.Logger.V(1).Info("failed to resolve trust material, will retry", "error", err)

	if cond != nil && cond.Reason == ReasonDrifted {
		return a.RequeueAfter(5 * time.Second)
	}

	instance.SetCondition(metav1.Condition{
		Type:               TrustMaterialCondition,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonResolveFailed,
		Message:            err.Error(),
		ObservedGeneration: instance.GetGeneration(),
	})
	if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
		return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), persistErr), instance)
	}
	return a.RequeueAfter(5 * time.Second)
}

// handleDrift flags a newly observed (or re-affirms an already flagged)
// trust material change and blocks Ready. Reason is preserved (never
// "Failure") since internal/state derives CanHandle gates from it.
func (a *resolveAction[T]) handleDrift(ctx context.Context, instance T, cond *metav1.Condition) *action.Result {
	// Already flagged — re-affirm without re-persisting or re-firing the event.
	if cond != nil && cond.Reason == ReasonDrifted {
		return a.Return()
	}

	message := fmt.Sprintf(
		"%s trust material changed. Verify this is expected, then annotate this resource with %s=\"true\" to accept it.",
		a.resolver.ComponentName(), annotations.RefreshTrustMaterial)

	instance.SetCondition(metav1.Condition{
		Type:               TrustMaterialCondition,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonDrifted,
		Message:            message,
		ObservedGeneration: instance.GetGeneration(),
	})
	a.setReadyBlocked(instance, fmt.Sprintf(
		"%s trust material changed and requires manual acknowledgement — see the %s condition",
		a.resolver.ComponentName(), TrustMaterialCondition))

	if _, err := a.PersistStatus(ctx, instance); err != nil {
		return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), err), instance)
	}

	a.Recorder.Eventf(instance, nil, "Warning", "TrustMaterialDrifted", ReasonDrifted, message)
	return a.Return()
}

// acceptTrustMaterial stores pem and marks the condition Resolved. wasDrifted
// picks the fired event: Updated for an accepted change, Resolved for
// first-time resolution, none if unchanged.
func (a *resolveAction[T]) acceptTrustMaterial(ctx context.Context, instance T, pem string, wasDrifted bool) *action.Result {
	current := a.resolver.GetTrustMaterial(instance)
	acknowledged := hasRefreshAcknowledgement(instance)
	a.resolver.SetTrustMaterial(instance, pem)

	instance.SetCondition(metav1.Condition{
		Type:               TrustMaterialCondition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            fmt.Sprintf("Trust material for %s resolved successfully", a.resolver.ComponentName()),
		ObservedGeneration: instance.GetGeneration(),
	})

	if _, err := a.PersistStatus(ctx, instance); err != nil {
		return a.Error(ctx, fmt.Errorf("%w: %s: %w", ErrPersistStatus, a.resolver.ComponentName(), err), instance)
	}

	switch {
	case current == "":
		a.Recorder.Eventf(instance, nil, "Normal", "TrustMaterialResolved",
			ReasonResolved, "Resolved %s trust material from running service", a.resolver.ComponentName())
	case wasDrifted:
		a.Recorder.Eventf(instance, nil, "Normal", "TrustMaterialUpdated", "Updated",
			"Accepted updated %s trust material after manual acknowledgement", a.resolver.ComponentName())
	}

	if acknowledged {
		if _, err := k8sutils.CreateOrUpdate(ctx, a.Client, instance,
			ensure.Annotations[T]([]string{annotations.RefreshTrustMaterial}, map[string]string{}),
		); err != nil {
			a.Logger.Error(err, "failed to clear trust material acknowledgement annotation", "component", a.resolver.ComponentName())
			a.Recorder.Eventf(instance, nil, "Warning", "AnnotationClearFailed", "Failed",
				"Accepted %s trust material but failed to clear the %s annotation: %s",
				a.resolver.ComponentName(), annotations.RefreshTrustMaterial, err.Error())
		}
	}

	return a.Continue()
}

// setReadyBlocked flips the shared Ready condition to Status=False while
// preserving whatever Reason it already had. Reason must stay a value
// state.FromReason recognizes (e.g. "Ready", "Initialize") — an unrecognized
// one (including "Failure" or "Drifted") maps to state.None and would break
// CanHandle gates keyed on a minimum state, like CTlog's own resolver.
func (a *resolveAction[T]) setReadyBlocked(instance T, message string) {
	readyReason := state.Ready.String()
	if existingReady := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition); existingReady != nil {
		readyReason = existingReady.Reason
	}
	instance.SetCondition(metav1.Condition{
		Type:               constants.ReadyCondition,
		Status:             metav1.ConditionFalse,
		Reason:             readyReason,
		Message:            message,
		ObservedGeneration: instance.GetGeneration(),
	})
}
