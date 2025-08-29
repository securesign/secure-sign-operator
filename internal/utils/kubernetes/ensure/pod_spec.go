package ensure

import (
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	runAsUser  int64 = 1001
	runAsGroup int64 = 1001
)

func PodSecurityContext(spec *core.PodSpec) error {
	if spec.SecurityContext == nil {
		spec.SecurityContext = &core.PodSecurityContext{}
	}
	spec.SecurityContext.RunAsNonRoot = ptr.To(true)
	spec.SecurityContext.FSGroupChangePolicy = ptr.To(core.FSGroupChangeOnRootMismatch)

	if spec.SecurityContext.SeccompProfile == nil {
		spec.SecurityContext.SeccompProfile = &core.SeccompProfile{}
	}
	spec.SecurityContext.SeccompProfile.Type = core.SeccompProfileTypeRuntimeDefault

	if !kubernetes.IsOpenShift() && spec.SecurityContext.FSGroup == nil {
		spec.SecurityContext.FSGroup = ptr.To(runAsGroup)
	}

	for i := range spec.InitContainers {
		container := &spec.InitContainers[i]

		if container.SecurityContext == nil {
			container.SecurityContext = &core.SecurityContext{}
		}

		if container.SecurityContext.RunAsNonRoot == nil {
			container.SecurityContext.RunAsNonRoot = ptr.To(true)
		}
		if container.SecurityContext.AllowPrivilegeEscalation == nil {
			container.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
		}
		if !kubernetes.IsOpenShift() && container.SecurityContext.RunAsUser == nil {
			container.SecurityContext.RunAsUser = ptr.To(runAsUser)
		}
	}

	for i := range spec.Containers {
		container := &spec.Containers[i]

		if container.SecurityContext == nil {
			container.SecurityContext = &core.SecurityContext{}
		}

		if container.SecurityContext.RunAsNonRoot == nil {
			container.SecurityContext.RunAsNonRoot = ptr.To(true)
		}
		if container.SecurityContext.AllowPrivilegeEscalation == nil {
			container.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
		}
		if !kubernetes.IsOpenShift() && container.SecurityContext.RunAsUser == nil {
			container.SecurityContext.RunAsUser = ptr.To(runAsUser)
		}
	}

	return nil
}
