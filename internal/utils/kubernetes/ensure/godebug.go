package ensure

import (
	"os"

	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
)

// GODEBUG propagates GODEBUG to all PodSpec containers, respecting the
// rhtas.redhat.com/godebug annotation for per-component override.
//
// Resolution order:
//  1. Annotation present and non-empty → use annotation value (override).
//  2. Annotation present and empty → remove existing GODEBUG and prevent propagation.
//  3. Annotation absent → fall back to the operator's own GODEBUG env var
//     (if the operator's GODEBUG is unset, any existing GODEBUG is removed).
func GODEBUG(componentAnnotations map[string]string) func(*core.PodSpec) error {
	return func(spec *core.PodSpec) error {
		annotationValue, annotationOk := componentAnnotations[annotations.Godebug]
		osEnvValue := os.Getenv("GODEBUG")

		managed := make([]core.Container, len(spec.Containers))
		for i, c := range spec.Containers {
			managed[i] = core.Container{
				Name: c.Name,
				Env:  []core.EnvVar{{Name: "GODEBUG"}},
			}
		}

		return OptionalToggle((annotationOk && annotationValue != "") || (!annotationOk && osEnvValue != ""), Toggleable[*core.PodSpec]{
			Ensure: func(spec *core.PodSpec) error {
				godebug := annotationValue
				if godebug == "" {
					godebug = osEnvValue
				}
				for i := range spec.Containers {
					env := kubernetes.FindEnvByNameOrCreate(&spec.Containers[i], "GODEBUG")
					env.Value = godebug
				}
				return nil
			},
			Managed: &core.PodSpec{
				Containers: managed,
			},
		})(spec)
	}
}
