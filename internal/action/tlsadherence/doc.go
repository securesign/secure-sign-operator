// Package tlsadherence provides a generic action that gates a component's
// rollout on its ability to honour the cluster TLS security profile.
//
// The effective mode combines the cluster-wide TLS adherence policy (a floor)
// with the per-resource rhtas.redhat.com/tlsAdherence annotation; see
// [resolveMode]. For a component that cannot honour the profile: strict blocks
// the rollout with a terminal error condition, legacy deploys it with a Warning
// condition, and a legacy annotation on a strict cluster is a configuration
// error (blocked).
//
// The action only runs when the operator resolves the cluster profile
// (see [github.com/securesign/operator/internal/config.ClusterTLSProfileEnforced]);
// otherwise it is a no-op. This is an interim control, removed once all upstream
// binaries honour the cluster profile.
package tlsadherence
