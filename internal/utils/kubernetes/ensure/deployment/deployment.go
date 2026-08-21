package deployment

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

func Proxy(noProxy ...string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		ensure.SetProxyEnvs(dp.Spec.Template.Spec.Containers, noProxy...)
		return nil
	}
}

func GODEBUG(componentAnnotations map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensure.GODEBUG(componentAnnotations)(&dp.Spec.Template.Spec)
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *rhtasv1.LocalObjectReference, containerName string, moreNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TrustedCA(lor, containerName, moreNames...)(&dp.Spec.Template)
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls rhtasv1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TLS(tls, containerNames...)(&dp.Spec.Template)
	}
}

func PodRequirements(requirements rhtasv1.PodRequirements, containerName string) func(*v1.Deployment) error {
	return func(deployment *v1.Deployment) error {
		deployment.Spec.Replicas = requirements.Replicas

		template := &deployment.Spec.Template
		template.Spec.Affinity = requirements.Affinity
		template.Spec.Tolerations = requirements.Tolerations

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		if requirements.Resources != nil {
			container.Resources = *requirements.Resources
		} else {
			container.Resources = core.ResourceRequirements{}
		}
		return nil
	}
}

// PodExtensions applies user-defined init containers, volumes, and volume mounts
// to a Deployment. It is a standalone ensure function so callers can compose it
// independently from signer-specific logic.
func PodExtensions(ext rhtasv1.PodExtensions, containerName string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		template := &dp.Spec.Template
		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		ensure.ReconcileUserPodResources(&template.Spec, container, ext)
		return nil
	}
}

func PodSecurityContext() func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensure.PodSecurityContext(&dp.Spec.Template.Spec)
	}
}

// Auth manages user-specified authentication configuration using UserSpecifiedToggle.
// This automatically removes env vars, volumes, and volume mounts when they're
// removed from the auth spec, unlike the old implementation which only added items.
func Auth(containerName string, auth *rhtasv1.Auth) func(*v1.Deployment) error {
	// Build managed state - sparse Deployment with only auth-managed fields
	managed := buildAuthManagedState(containerName, auth)

	toggle := ensure.Toggleable[*v1.Deployment]{
		Ensure: func(d *v1.Deployment) error {
			// Use existing Auth logic to add/update items
			return ensure.Auth(containerName, auth)(&d.Spec.Template.Spec)
		},
		Managed: managed,
	}

	return ensure.UserSpecifiedToggle(toggle)
}

// buildAuthManagedState creates a sparse Deployment declaring what Auth manages.
// Always declares container env vars and auth volume/mount (even if empty) so
// UserSpecifiedToggle knows to clean them up when removed from spec.
func buildAuthManagedState(containerName string, auth *rhtasv1.Auth) *v1.Deployment {
	// Initialize as empty slices (not nil) so projection knows we manage these fields
	envVars := []core.EnvVar{}
	volumeMounts := []core.VolumeMount{}
	volumes := []core.Volume{}

	if auth != nil {
		envVars = append([]core.EnvVar{}, auth.Env...) // Copy to avoid sharing

		// Build volume mount and volume if secrets exist
		if len(auth.SecretMount) > 0 {
			volumeMounts = []core.VolumeMount{
				{
					Name:      "auth",
					MountPath: ensure.AuthMountPath,
					ReadOnly:  true,
				},
			}

			sources := make([]core.VolumeProjection, 0, len(auth.SecretMount))
			for _, secret := range auth.SecretMount {
				sources = append(sources, core.VolumeProjection{
					Secret: &core.SecretProjection{
						LocalObjectReference: core.LocalObjectReference{Name: secret.Name},
					},
				})
			}
			volumes = []core.Volume{
				{
					Name: "auth",
					VolumeSource: core.VolumeSource{
						Projected: &core.ProjectedVolumeSource{Sources: sources},
					},
				},
			}
		}
	}

	// Always declare the container to ensure cleanup works even when auth is nil
	container := core.Container{
		Name:         containerName,
		Env:          envVars,      // Empty slice (not nil) when auth is nil
		VolumeMounts: volumeMounts, // Empty slice (not nil) when no secrets
	}

	return &v1.Deployment{
		Spec: v1.DeploymentSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{container},
					Volumes:    volumes, // Empty slice (not nil) when no secrets
				},
			},
		},
	}
}
