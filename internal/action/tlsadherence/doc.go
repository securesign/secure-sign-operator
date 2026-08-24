// Package tlsadherence provides a generic action that gates a component's
// rollout on its ability to honour the cluster TLS security profile.
//
// # Effective mode
//
// The outcome is driven by the effective enforcement mode, which combines two
// inputs:
//
//   - the cluster-wide TLS adherence policy (OpenShift APIServer
//     spec.tlsAdherence), snapshotted at startup into
//     [github.com/securesign/operator/internal/config.ClusterTLSAdherenceStrict];
//   - the per-resource rhtas.redhat.com/tlsAdherence annotation
//     (see [github.com/securesign/operator/internal/annotations.TLSAdherence]).
//
// The cluster policy is a floor: the annotation may match or tighten it, never
// relax it. Given cluster policy P and annotation A:
//
//	P strict     + A absent  -> strict   (inherit the cluster mandate)
//	P strict     + A strict   -> strict
//	P strict     + A legacy   -> configuration error (relaxing below the floor)
//	P not-strict + A absent   -> legacy   (operator default)
//	P not-strict + A legacy   -> legacy
//	P not-strict + A strict   -> strict   (operator opts in tighter)
//
// StrictAllComponents (or any unrecognised value of a present field) is strict;
// NoOpinion and LegacyAdheringComponentsOnly are not. On most clusters the field
// is absent or the TLSAdherence feature gate is off, which resolves to NoOpinion
// (not strict), so the annotation is the operative control there.
//
// # Outcomes (for a component that cannot honour the profile)
//
//   - strict: blocked with a terminal error condition; not rolled out.
//   - legacy: deployed anyway with a Warning condition; rollout is not blocked.
//   - configuration error: the annotation relaxes below the cluster floor;
//     blocked with a terminal error condition (reason InvalidConfiguration).
//
// A component that can honour the profile is a no-op regardless of mode.
//
// # Disabled resolution
//
// The action only takes effect when the operator resolves and enforces the
// cluster TLS security profile (OpenShift with resolution enabled; see
// [github.com/securesign/operator/internal/config.ClusterTLSProfileEnforced]). On
// vanilla Kubernetes, or when DisableClusterTLSProfile is set, the action is a
// no-op: no condition is set and the rollout is never blocked.
//
// # Capability check
//
// Whether a component can honour the profile is decided by the per-component
// TLS-capability check passed to [NewAction]. Until the upstream binaries
// expose that signal, [CanHonourClusterTLSProfile] is a stub that always
// reports compliant (see STORY-994), so the action is a no-op in production
// and only exercised in unit tests.
//
// This is an interim control; once all upstream binaries honour the cluster
// profile it will be deprecated and removed.
package tlsadherence
