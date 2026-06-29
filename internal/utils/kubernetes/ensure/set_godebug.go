package ensure

import (
	"os"

	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
)

// SetGodebugEnv propagates GODEBUG to all containers, respecting the
// rhtas.redhat.com/godebug annotation for per-component override.
//
// Resolution order:
//  1. Annotation present and non-empty → use annotation value (override).
//  2. Annotation present and empty → remove existing GODEBUG and prevent propagation.
//  3. Annotation absent → fall back to the operator's own GODEBUG env var
//     (if the operator's GODEBUG is also empty, any existing GODEBUG is removed).
//
// Only Spec.Containers are passed in — init containers are intentionally excluded
// because current init containers are shell/netcat utilities, not Go binaries.
// If a Go-based init container is added in the future, it must be included here.
func SetGodebugEnv(containers []v1.Container, componentAnnotations map[string]string) {
	godebug, ok := componentAnnotations[annotations.Godebug]
	if ok && godebug == "" {
		for i := range containers {
			kubernetes.RemoveEnvVarByName(&containers[i], "GODEBUG")
		}
		return
	}
	if !ok {
		godebug = os.Getenv("GODEBUG")
	}
	if godebug == "" {
		for i := range containers {
			kubernetes.RemoveEnvVarByName(&containers[i], "GODEBUG")
		}
		return
	}
	for i := range containers {
		env := kubernetes.FindEnvByNameOrCreate(&containers[i], "GODEBUG")
		env.Value = godebug
	}
}
