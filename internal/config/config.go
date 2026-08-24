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
	// policy mandates that all components honour the cluster TLS security profile.
	// Resolved once at startup, it acts as a floor: a per-resource tlsAdherence
	// annotation may match or tighten it but never relax it.
	ClusterTLSAdherenceStrict bool
)

// ClusterTLSProfileEnforced reports whether the operator resolves and enforces
// the cluster-wide TLS security profile. True only on OpenShift with resolution
// enabled; otherwise callers must treat the cluster TLS profile as unavailable.
func ClusterTLSProfileEnforced() bool {
	return Openshift && !DisableClusterTLSProfile
}
