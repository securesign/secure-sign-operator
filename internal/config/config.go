package config

import "time"

var (
	CreateTreeDeadline       int64 = 1200
	Openshift                bool
	OpenshiftAPIServerName   string
	APIServerTimeout         time.Duration
	IngressHostTemplate      = "%[1]s.local"
	DisableClusterTLSProfile bool

	// ClusterTLSAdherenceStrict reports whether the cluster-wide TLS adherence
	// policy mandates that all components honour the cluster TLS security profile
	// (OpenShift APIServer spec.tlsAdherence = StrictAllComponents, or any unknown
	// value, which is treated as strict per openshift/api). It is resolved once at
	// startup and acts as a floor: a per-resource tlsAdherence annotation may match
	// or tighten it but never relax it.
	ClusterTLSAdherenceStrict bool
)

// ClusterTLSProfileEnforced reports whether the operator resolves and enforces
// the cluster-wide TLS security profile. It is only true on OpenShift and when
// resolution has not been disabled via DisableClusterTLSProfile. On vanilla
// Kubernetes or when disabled, callers must treat the cluster TLS profile as
// unavailable and behave exactly as they did before it existed.
func ClusterTLSProfileEnforced() bool {
	return Openshift && !DisableClusterTLSProfile
}
