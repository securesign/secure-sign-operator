package ensure

import (
	"slices"

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
// volume mounts to a PodSpec and main container. This is the shared entry point
// called by both Fulcio and CTLog deployment actions to avoid duplication.
func ReconcileUserPodResources(podSpec *core.PodSpec, container *core.Container, ext v1.PodExtensions) {
	ReconcileInitContainers(podSpec, ext.InitContainers)

	for i := range ext.Volumes {
		coreVol := ext.Volumes[i].ToVolume()
		v := kubernetes.FindVolumeByNameOrCreate(podSpec, coreVol.Name)
		v.VolumeSource = coreVol.VolumeSource
	}

	for _, vm := range ext.VolumeMounts {
		m := kubernetes.FindVolumeMountByNameOrCreate(container, vm.Name)
		name := m.Name
		*m = vm
		m.Name = name
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
		// Only set ImagePullPolicy if explicitly specified; otherwise preserve
		// the Kubernetes default applied by the API server (e.g. "IfNotPresent").
		// The Go zero value ("") differs from the API server default and would
		// cause an infinite update loop via CreateOrUpdate.
		if spec.ImagePullPolicy != "" {
			c.ImagePullPolicy = spec.ImagePullPolicy
		}
		c.RestartPolicy = spec.RestartPolicy
		// Use slices.Clone instead of append([]T{}, slice...) to preserve nil.
		// append([]T{}, nil...) produces a non-nil empty slice ([]T{}), but the
		// Kubernetes API server strips empty slices via omitempty on round-trip,
		// returning nil. reflect.DeepEqual(nil, []T{}) is false, causing
		// CreateOrUpdate to "update" the Deployment on every reconcile -- an
		// infinite loop that demotes Ready from Initialize back to Creating.
		c.Env = slices.Clone(spec.Env)
		c.VolumeMounts = slices.Clone(spec.VolumeMounts)
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
