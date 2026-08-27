package tlsadherence

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewAction returns a generic action that gates a component's rollout on its
// ability to honour the cluster TLS security profile. canHonourProfile is the
// per-component capability check; when it reports false the effective mode
// (cluster policy combined with the tlsAdherence annotation) decides the outcome.
// See the package doc for the full behaviour. The action is a no-op unless
// cluster TLS profile resolution is enforced.
func NewAction[T apis.ConditionsAwareObject](component string, canHonourProfile func(T) bool) action.Action[T] {
	return &adherenceAction[T]{component: component, canHonourProfile: canHonourProfile}
}

type adherenceAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	component        string
	canHonourProfile func(T) bool
}

func (i adherenceAction[T]) Name() string {
	return fmt.Sprintf("check %s TLS profile adherence", i.component)
}

// CanHandle fires when cluster TLS profile resolution is enforced and the
// component cannot honour the profile, or when a stale TLSProfileAdherence
// condition still needs cleaning up (e.g. after the CLI flag is disabled).
func (i adherenceAction[T]) CanHandle(_ context.Context, instance T) bool {
	// A leftover condition must be removed even when the component is no longer
	// gated, so always handle while one is present.
	if meta.FindStatusCondition(instance.GetConditions(), TLSProfileAdherenceCondition) != nil {
		return true
	}
	if !config.ClusterTLSProfileEnforced() {
		return false
	}
	return !i.canHonourProfile(instance)
}

func (i adherenceAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	// No longer gated (resolution disabled or now compliant): remove any stale
	// condition left from a previous reconcile.
	if !config.ClusterTLSProfileEnforced() || i.canHonourProfile(instance) {
		instance.RemoveCondition(TLSProfileAdherenceCondition)
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	// Non-compliant component on a cluster that enforces the profile: combine the
	// cluster-wide adherence policy (a floor) with the per-resource annotation to
	// decide the effective enforcement mode.
	effective, conflict := resolveMode(config.ClusterTLSAdherenceStrict, intentFromAnnotations(instance.GetAnnotations()))

	msg := fmt.Sprintf("component %s cannot honour the cluster TLS security profile", i.component)

	// A legacy annotation on a strict cluster relaxes below the cluster mandate:
	// surface it as a configuration error.
	if conflict {
		err := reconcile.TerminalError(fmt.Errorf("%s: tlsAdherence=legacy conflicts with the cluster TLS adherence policy (strict); the annotation may not relax below the cluster mandate", msg))
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:               TLSProfileAdherenceCondition,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonInvalidConfiguration,
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	if effective == ModeStrict {
		err := reconcile.TerminalError(fmt.Errorf("%s: rollout blocked because the effective TLS adherence mode is strict", msg))
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:               TLSProfileAdherenceCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	instance.SetCondition(metav1.Condition{
		Type:               TLSProfileAdherenceCondition,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonWarning,
		Message:            fmt.Sprintf("%s; deploying anyway (effective TLS adherence mode is legacy)", msg),
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
