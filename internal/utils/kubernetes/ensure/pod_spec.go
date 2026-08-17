package ensure

import (
	v1 "github.com/securesign/operator/api/v1"
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

	if !kubernetes.IsOpenShift() {
		if spec.SecurityContext.FSGroup == nil {
			spec.SecurityContext.FSGroup = ptr.To(runAsGroup)
		}
	} else {
		spec.SecurityContext.FSGroup = nil
	}

	for i := range spec.InitContainers {
		ensureContainerSecurityContext(&spec.InitContainers[i])
	}

	for i := range spec.Containers {
		ensureContainerSecurityContext(&spec.Containers[i])
	}

	return nil
}

func ensureContainerSecurityContext(container *core.Container) {
	if container.SecurityContext == nil {
		container.SecurityContext = &core.SecurityContext{}
	}

	if container.SecurityContext.RunAsNonRoot == nil {
		container.SecurityContext.RunAsNonRoot = ptr.To(true)
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil {
		container.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
	}
	if container.SecurityContext.Capabilities == nil {
		container.SecurityContext.Capabilities = &core.Capabilities{}
	}
	if container.SecurityContext.Capabilities.Drop == nil {
		container.SecurityContext.Capabilities.Drop = []core.Capability{"ALL"}
	}
	if !kubernetes.IsOpenShift() {
		if container.SecurityContext.RunAsUser == nil {
			container.SecurityContext.RunAsUser = ptr.To(runAsUser)
		}
	} else {
		container.SecurityContext.RunAsUser = nil
	}
}

// ReconcileUserPodResources applies user-defined init containers, volumes, and
// volume mounts to a PodSpec. It uses the previous UserSpecState to remove
// volumes and volume mounts that are no longer desired via EnsureWithCleanup.
// Returns the current state so the caller can persist it via WriteLastApplied.
func ReconcileUserPodResources(podSpec *core.PodSpec, containerName string, ext v1.PodExtensions, prev UserSpecState) (UserSpecState, error) {
	ReconcileInitContainers(podSpec, ext.InitContainers)

	if err := EnsureWithCleanup(podSpec, Toggleable[*core.PodSpec]{
		Ensure:  upsertUserResources(containerName, ext),
		Managed: StaleResources(prev, ext, containerName),
	}); err != nil {
		return UserSpecState{}, err
	}

	return UserSpecState{
		Volumes:      Names(ext.Volumes, func(v v1.AdditionalVolume) string { return v.Name }),
		VolumeMounts: Names(ext.VolumeMounts, func(vm core.VolumeMount) string { return vm.Name }),
	}, nil
}

func upsertUserResources(containerName string, ext v1.PodExtensions) func(*core.PodSpec) error {
	return func(spec *core.PodSpec) error {
		for i := range ext.Volumes {
			coreVol := ext.Volumes[i].ToVolume()
			v := kubernetes.FindVolumeByNameOrCreate(spec, coreVol.Name)
			v.VolumeSource = coreVol.VolumeSource
		}

		container := kubernetes.FindContainerByNameOrCreate(spec, containerName)
		for _, vm := range ext.VolumeMounts {
			m := kubernetes.FindVolumeMountByNameOrCreate(container, vm.Name)
			name := m.Name
			*m = vm
			m.Name = name
		}
		return nil
	}
}

// ReconcileInitContainers reconciles user-defined init containers on a PodSpec.
// Each spec is upserted by name using FindInitContainerByNameOrCreate to preserve
// Kubernetes-defaulted fields (TerminationMessagePath, ImagePullPolicy).
// Init containers whose names are no longer in the desired list are removed.
func ReconcileInitContainers(podSpec *core.PodSpec, specs []v1.InitContainerSpec) {
	desiredNames := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		desiredNames[spec.Name] = struct{}{}
		c := kubernetes.FindInitContainerByNameOrCreate(podSpec, spec.Name)
		c.Image = spec.Image
		c.Command = spec.Command
		c.Args = spec.Args
		c.EnvFrom = spec.EnvFrom
		c.SecurityContext = spec.SecurityContext
		if spec.Resources != nil {
			c.Resources = *spec.Resources
		} else {
			c.Resources = core.ResourceRequirements{}
		}
		if spec.ImagePullPolicy != "" {
			c.ImagePullPolicy = spec.ImagePullPolicy
		}
		c.RestartPolicy = spec.RestartPolicy
		c.Env = append([]core.EnvVar{}, spec.Env...)
		c.VolumeMounts = append([]core.VolumeMount{}, spec.VolumeMounts...)
	}

	// Remove init containers that are no longer in the spec.
	if len(desiredNames) == 0 {
		podSpec.InitContainers = nil
		return
	}
	filtered := make([]core.Container, 0, len(podSpec.InitContainers))
	for _, c := range podSpec.InitContainers {
		if _, ok := desiredNames[c.Name]; ok {
			filtered = append(filtered, c)
		}
	}
	podSpec.InitContainers = filtered
}
