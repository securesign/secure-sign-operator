package tlsadherence

import (
	"github.com/securesign/operator/internal/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Mode selects how the operator reacts when a managed component cannot honour
// the cluster TLS security profile.
type Mode string

const (
	// ModeLegacy deploys a non-compliant component and surfaces a Warning
	// condition without blocking the rollout.
	ModeLegacy Mode = "legacy"

	// ModeStrict blocks the rollout of a non-compliant component and surfaces
	// an error condition instead.
	ModeStrict Mode = "strict"
)

// TLSProfileAdherenceCondition is the status condition type reporting a
// component's adherence to the cluster TLS security profile.
const TLSProfileAdherenceCondition = "TLSProfileAdherence"

// ReasonWarning is the condition reason when a non-compliant component is
// deployed anyway under legacy mode.
const ReasonWarning = "Warning"

// ReasonInvalidConfiguration is the condition reason when the tlsAdherence
// annotation tries to relax below the cluster-wide TLS adherence policy floor.
const ReasonInvalidConfiguration = "InvalidConfiguration"

// annotationIntent captures the three meaningful states of the tlsAdherence
// annotation. Absent is distinct from an explicit "legacy": on a strict cluster
// absent inherits the cluster mandate, whereas "legacy" tries to relax below it.
type annotationIntent int

const (
	annotationAbsent annotationIntent = iota
	annotationLegacy
	annotationStrict
)

// intentFromAnnotations classifies the tlsAdherence annotation. An unrecognised
// value is treated as absent, so a typo defaults to the cluster's own policy.
func intentFromAnnotations(a map[string]string) annotationIntent {
	switch a[annotations.TLSAdherence] {
	case string(ModeStrict):
		return annotationStrict
	case string(ModeLegacy):
		return annotationLegacy
	default:
		return annotationAbsent
	}
}

// resolveMode combines the cluster-wide TLS adherence policy (a floor) with the
// per-resource tlsAdherence annotation to produce the effective enforcement mode.
// The annotation may match or tighten the floor, never relax it: on a strict
// cluster an explicit "legacy" is a conflict, while absent or "strict" stays
// strict; on a non-strict cluster "strict" tightens and everything else is legacy.
func resolveMode(clusterStrict bool, intent annotationIntent) (effective Mode, conflict bool) {
	if clusterStrict {
		return ModeStrict, intent == annotationLegacy
	}
	if intent == annotationStrict {
		return ModeStrict, false
	}
	return ModeLegacy, false
}

// CanHonourClusterTLSProfile reports whether the component's upstream binary can
// honour the cluster TLS security profile.
//
// TODO: replace this stub with a real per-component TLS-capability signal. Until
// then every component is assumed compliant, so the annotation is a no-op in
// production and only exercised in unit tests.
func CanHonourClusterTLSProfile[T client.Object](_ T) bool {
	return true
}
