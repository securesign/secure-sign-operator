// Package annotations provides keys for Kubernetes annotations used to configure
// and modify the behavior of the operator and its managed resources.
//
// # Annotation: rhtas.redhat.com/pause-reconciliation
//
// [PausedReconciliation] pauses the reconciliation of any managed Kubernetes resource.
//
// Note: Use with caution, as paused resources will not receive updates from the operator.
//
// Options:
//   - "true": Disables reconciliation by the operator.
//   - "false": Enables reconciliation by the operator.
//
// Example usage:
//
//	apiVersion: app/v1
//	kind: Deployment
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/pause-reconciliation: "true"
//
// # Annotation: rhtas.redhat.com/trusted-ca
//
// [TrustedCA] specifies the name of a ConfigMap containing a custom CA bundle.
//
// If set on the Securesign resource, this annotation is automatically propagated
// to child resources. ([github.com/securesign/operator/api/v1.Securesign])
//
// Example usage:
//
//	---
//	apiVersion: v1
//	kind: ConfigMap
//	metadata:
//	  name: custom-ca-bundle
//	data:
//	  ca-bundle.crt: ...
//	---
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/trusted-ca: "custom-ca-bundle"
//	---
//
// # Annotation: rhtas.redhat.com/godebug
//
// [Godebug] overrides the GODEBUG environment variable propagated to managed containers.
//
// By default, the operator propagates its own GODEBUG value to all managed workloads.
// This annotation allows control over GODEBUG propagation:
//   - Not set: inherit the operator's GODEBUG value (default behavior).
//   - Set to a value (e.g. "fips140=only"): use that value instead.
//   - Set to empty string "": disable GODEBUG propagation and remove any existing GODEBUG env var.
//
// If set on the Securesign resource, this annotation is automatically propagated
// to all child resources and overwrites any value set directly on them.
// Per-component overrides via this annotation are only effective on standalone
// child CRs that are not managed by a Securesign parent.
// ([github.com/securesign/operator/api/v1.Securesign])
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/godebug: "fips140=only"
//
// # Annotation: rhtas.redhat.com/refresh-trust-material
//
// [RefreshTrustMaterial] acknowledges a detected change in a component's trust
// material (public key or certificate) and instructs the operator to accept the
// newly observed value.
//
// Components may use an external KMS/HSM/Tink signer, so the operator can only
// observe the current public key/certificate by asking the running service —
// it fetches this on every reconcile and caches it in the component's status for
// autodiscovery by TUF and other components. If the fetched value ever differs
// from the cached one (for example, after rotating a key in an external KMS),
// the operator does not update the status automatically: blindly accepting the
// new value could break verification of artifacts signed with the old key,
// since the transparency-log tree and TUF trust metadata also need to be
// rotated through the documented key-rotation procedure (see docs/*-key-rotation.md).
//
// Once the required manual rotation steps have been completed, set this
// annotation to "true" to have the operator accept the newly observed trust
// material. The operator removes the annotation after processing it.
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Rekor
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/refresh-trust-material: "true"
//
// # Annotation: rhtas.redhat.com/log-type
//
// [LogType] specifies the logging configuration for managed services.
//
// If not set, the logging configuration defaults to "prod" type.
//
// Supported logging types:
//   - "dev": Enables verbose logging for debugging purposes.
//   - "prod": Enables minimal, structured logging optimized for performance.
//
// Affects the following services:
//   - Rekor ([github.com/securesign/operator/api/v1.Rekor])
//   - Timestamp Authority ([github.com/securesign/operator/api/v1.TimestampAuthority])
//   - Fulcio ([github.com/securesign/operator/api/v1.Fulcio])
//
// If set on the Securesign resource, this annotation is automatically propagated
// to child resources. ([github.com/securesign/operator/api/v1.Securesign])
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/log-type: "dev"
//
// # Annotation: rhtas.redhat.com/tlsAdherence
//
// [TLSAdherence] controls whether the operator blocks or only warns when a
// managed component cannot yet honour the cluster TLS security profile. This is
// an interim control, removed once all upstream binaries honour the profile.
//
// Supported values:
//   - "legacy": deploy a non-compliant component with a Warning condition; do
//     not block. This is the default when the cluster policy is not strict.
//   - "strict": block deployment of a non-compliant component with an error
//     condition.
//
// Any other value is treated as absent, so the component inherits the cluster
// policy. The cluster-wide policy (OpenShift APIServer spec.tlsAdherence) is a
// floor: this annotation may match or tighten it but never relax it, so "legacy"
// on a strict cluster is a configuration error.
//
// The annotation only takes effect on clusters where the operator resolves the
// cluster TLS profile (OpenShift with resolution enabled); elsewhere it has no
// effect. If set on the Securesign resource it is propagated to child resources
// and overwrites any value set directly on them; per-component overrides are only
// effective on standalone child CRs.
// ([github.com/securesign/operator/api/v1.Securesign])
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/tlsAdherence: "strict"
package annotations

const (
	// PausedReconciliation defines the annotation key used to pause reconciliation for a resource.
	PausedReconciliation = "rhtas.redhat.com/pause-reconciliation"

	// TrustedCA defines the annotation key for specifying a custom CA bundle ConfigMap.
	TrustedCA = "rhtas.redhat.com/trusted-ca"

	// Godebug defines the annotation key for overriding the GODEBUG environment variable per component.
	Godebug = "rhtas.redhat.com/godebug"

	// LogType defines the annotation key used to configure the logging type for managed resources.
	LogType = "rhtas.redhat.com/log-type"

	// TreeId define the annotation key to document association of resource with specific Merkle Tree
	TreeId = "rhtas.redhat.com/treeId"

	// RefreshTrustMaterial defines the annotation key used to acknowledge a detected
	// trust material change and accept the newly observed value.
	RefreshTrustMaterial = "rhtas.redhat.com/refresh-trust-material"

	// PKCS11SpecHash stores the SHA-256 hash of the resolved PKCS#11 spec fields
	// for drift detection across reconcile cycles.
	PKCS11SpecHash = "rhtas.redhat.com/pkcs11-spec-hash"

	// CreateTreeAttempts counts consecutive failed createtree Jobs, to bound retries.
	CreateTreeAttempts = "rhtas.redhat.com/createtree-attempts"

	// CreateTreeAttemptsGeneration is the CR generation CreateTreeAttempts applies to.
	CreateTreeAttemptsGeneration = "rhtas.redhat.com/createtree-attempts-generation"

	// CreateTreeNextRetry is the earliest RFC3339 time the failed Job may be recreated.
	CreateTreeNextRetry = "rhtas.redhat.com/createtree-next-retry"

	// TLSAdherence defines the annotation key controlling whether the operator blocks
	// ("strict") or only warns ("legacy", the default) when a component cannot yet
	// honour the cluster TLS security profile.
	TLSAdherence = "rhtas.redhat.com/tlsAdherence"

	TLS = "service.beta.openshift.io/serving-cert-secret-name"
)

var InheritableAnnotations = []string{
	TrustedCA, LogType, Godebug, TLSAdherence,
}
