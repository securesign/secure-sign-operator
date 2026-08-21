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
// to child resources. ([github.com/securesign/operator/api/rhtasv1.Securesign])
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
// ([github.com/securesign/operator/api/rhtasv1.Securesign])
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
//   - Rekor ([github.com/securesign/operator/api/rhtasv1.Rekor])
//   - Timestamp Authority ([github.com/securesign/operator/api/rhtasv1.TimestampAuthority])
//   - Fulcio ([github.com/securesign/operator/api/rhtasv1.Fulcio])
//
// If set on the Securesign resource, this annotation is automatically propagated
// to child resources. ([github.com/securesign/operator/api/rhtasv1.Securesign])
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/log-type: "dev"
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

	// LastUserSpecApplied tracks, per concern, the names of user-specified
	// resources applied during the last reconcile, so the next reconcile can
	// detect removals. The value is a JSON object keyed by concern name, e.g.:
	//
	//	{"podExtensions": {"volumes": ["a"], "volumeMounts": ["a"]}}
	//
	// Each concern (e.g. "podExtensions") owns exactly one top-level key and
	// only ever reads/writes it via ensure.ReadNamespacedState /
	// ensure.WriteNamespacedState — this lets multiple independent ensure
	// functions share this one annotation on the same object without
	// overwriting each other's tracked state.
	//
	// We use an annotation-based diff approach instead of Server-Side Apply (SSA)
	// because SSA field ownership tracks at the field level, not the semantic
	// "user-specified resource" level. When the operator and user both touch
	// PodSpec.Volumes, SSA cannot distinguish operator-managed volumes from
	// user-specified ones — conflicts arise on shared slice fields, and removing
	// a single user volume requires the operator to re-apply the entire field,
	// which risks clobbering operator-managed entries. This annotation sidesteps
	// SSA ownership entirely: it records which named resources the user specified,
	// so the next reconcile can compute the diff and remove stale items without
	// affecting operator-managed resources in the same slice.
	LastUserSpecApplied = "rhtas.redhat.com/last-user-spec-applied"

	TLS = "service.beta.openshift.io/serving-cert-secret-name"
)

var InheritableAnnotations = []string{
	TrustedCA, LogType, Godebug,
}
