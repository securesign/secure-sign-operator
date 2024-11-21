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
// # Annotation: rhtas.redhat.com/metrics
//
// [Metrics] controls whether analytic metrics are collected for installed services.
// This annotation applies only to the Securesign resource.
//
// Options:
//   - "true": Enables metrics collection (default).
//   - "false": Disables metrics collection.
//
// Example usage:
//
//	apiVersion: rhtas.redhat.com/v1alpha1
//	kind: Securesign
//	metadata:
//	  name: example
//	  annotations:
//	    rhtas.redhat.com/metrics: "false"
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
package annotations

const (
	// PausedReconciliation defines the annotation key used to pause reconciliation for a resource.
	PausedReconciliation = "rhtas.redhat.com/pause-reconciliation"

	// Metrics defines the annotation key used to enable or disable metric collection by the operator.
	Metrics = "rhtas.redhat.com/metrics"

	// TrustedCA defines the annotation key for specifying a custom CA bundle ConfigMap.
	TrustedCA = "rhtas.redhat.com/trusted-ca"

	// TreeId define the annotation key to document association of resource with specific Merkle Tree
	TreeId = "rhtas.redhat.com/treeId"

	TLS = "service.beta.openshift.io/serving-cert-secret-name"
)

var inheritable = []string{
	TrustedCA,
}

func FilterInheritable(annotations map[string]string) map[string]string {
	result := make(map[string]string, 0)
	for key, value := range annotations {
		for _, ia := range inheritable {
			if key == ia {
				result[key] = value
			}
		}
	}
	return result
}
