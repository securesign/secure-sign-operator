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
// to child resources. ([github.com/securesign/operator/api/v1alpha1.Securesign])
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
//   - Rekor ([github.com/securesign/operator/api/v1alpha1.Rekor])
//   - Timestamp Authority ([github.com/securesign/operator/api/v1alpha1.TimestampAuthority])
//   - Fulcio ([github.com/securesign/operator/api/v1alpha1.Fulcio])
//
// If set on the Securesign resource, this annotation is automatically propagated
// to child resources. ([github.com/securesign/operator/api/v1alpha1.Securesign])
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

	// LogType defines the annotation key used to configure the logging type for managed resources.
	LogType = "rhtas.redhat.com/log-type"

	// TreeId define the annotation key to document association of resource with specific Merkle Tree
	TreeId = "rhtas.redhat.com/treeId"

	TLS = "service.beta.openshift.io/serving-cert-secret-name"
)

var InheritableAnnotations = []string{
	TrustedCA, LogType,
}
