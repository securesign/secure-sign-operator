package tlsadherence

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewAction returns a generic action that gates a component's rollout on its
// ability to honour the cluster TLS security profile.
//
// canHonourProfile is the per-component TLS-capability check. When it reports
// true (the compliant case) the action is a no-op. When it reports false the
// effective enforcement mode decides the outcome:
//   - strict blocks the rollout with a terminal error condition;
//   - legacy surfaces a Warning condition and lets the rollout continue.
//
// The effective mode is the cluster-wide TLS adherence policy combined with the
// per-resource rhtas.redhat.com/tlsAdherence annotation. The cluster policy is a
// floor: the annotation may match or tighten it, never relax it. A legacy
// annotation on a strict cluster is a configuration conflict and is blocked.
//
// The action only takes effect when cluster TLS profile resolution is enforced
// (see [github.com/securesign/operator/internal/config.ClusterTLSProfileEnforced]).
// On vanilla Kubernetes or when resolution is disabled it is a no-op: no
// condition is set and the rollout is never blocked.
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

// CanHandle fires only when cluster TLS profile resolution is enforced and the
// component cannot honour the profile. Compliant components, and any component on
// a cluster where resolution is disabled (vanilla Kubernetes or
// DisableClusterTLSProfile), are a no-op.
func (i adherenceAction[T]) CanHandle(_ context.Context, instance T) bool {
	if !config.ClusterTLSProfileEnforced() {
		return false
	}
	return !i.canHonourProfile(instance)
}

func (i adherenceAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	// Only reached for a non-compliant component on a cluster that enforces the
	// TLS profile. Combine the cluster-wide adherence policy (a floor) with the
	// per-resource annotation to decide the effective enforcement mode.
	effective, conflict := resolveMode(config.ClusterTLSAdherenceStrict, intentFromAnnotations(instance.GetAnnotations()))

	msg := fmt.Sprintf("component %s cannot honour the cluster TLS security profile", i.component)

	// A legacy annotation on a strict cluster tries to relax below the cluster
	// mandate: surface it as a configuration error rather than silently upgrading.
	if conflict {
		err := reconcile.TerminalError(fmt.Errorf("%s: tlsAdherence=legacy conflicts with the cluster TLS adherence policy (StrictAllComponents); the annotation may not relax below the cluster mandate", msg))
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
