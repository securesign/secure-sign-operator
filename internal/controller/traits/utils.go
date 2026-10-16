package traits

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	runAsUser  int64 = 1001
	runAsGroup int64 = 1001
)

// ConvertViaJSON converts between API types with matching JSON representations.
// This is primarily useful for converting Kubernetes API values to their
// generated apply-configuration counterparts.
func ConvertViaJSON[From, To any](from From) (*To, error) {
	data, err := json.Marshal(from)
	if err != nil {
		return nil, fmt.Errorf("marshal for apply configuration conversion: %w", err)
	}

	to := new(To)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(to); err != nil {
		return nil, fmt.Errorf("unmarshal for apply configuration conversion: %w", err)
	}

	return to, nil
}

// SSAPodSecurityContext returns the standard pod security context.
func SSAPodSecurityContext() *corev1ac.PodSecurityContextApplyConfiguration {
	sc := corev1ac.PodSecurityContext().
		WithRunAsNonRoot(true).
		WithFSGroupChangePolicy(core.FSGroupChangeOnRootMismatch).
		WithSeccompProfile(corev1ac.SeccompProfile().
			WithType(core.SeccompProfileTypeRuntimeDefault))

	if !kubernetes.IsOpenShift() {
		sc = sc.WithFSGroup(runAsGroup)
	}
	return sc
}

// SSAContainerSecurityContext returns the standard container security context.
func SSAContainerSecurityContext() *corev1ac.SecurityContextApplyConfiguration {
	sc := corev1ac.SecurityContext().
		WithRunAsNonRoot(true).
		WithAllowPrivilegeEscalation(false).
		WithCapabilities(corev1ac.Capabilities().
			WithDrop(core.Capability("ALL")))

	if !kubernetes.IsOpenShift() {
		sc = sc.WithRunAsUser(runAsUser)
	}
	return sc
}
