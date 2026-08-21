package cronjob

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	batchv1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
)

// Auth manages user-specified authentication configuration using UserSpecifiedToggle.
// This automatically removes env vars, volumes, and volume mounts when they're
// removed from the auth spec, unlike the old implementation which only added items.
func Auth(containerName string, auth *rhtasv1.Auth) func(*batchv1.CronJob) error {
	// Build managed state - sparse CronJob with only auth-managed fields
	managed := buildAuthManagedState(containerName, auth)

	toggle := ensure.Toggleable[*batchv1.CronJob]{
		Ensure: func(c *batchv1.CronJob) error {
			// Use existing Auth logic to add/update items
			return ensure.Auth(containerName, auth)(&c.Spec.JobTemplate.Spec.Template.Spec)
		},
		Managed: managed,
	}

	return ensure.UserSpecifiedToggle(toggle)
}

// buildAuthManagedState creates a sparse CronJob declaring what Auth manages.
// Always declares container env vars and auth volume/mount (even if empty) so
// UserSpecifiedToggle knows to clean them up when removed from spec.
func buildAuthManagedState(containerName string, auth *rhtasv1.Auth) *batchv1.CronJob {
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

	return &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: core.PodTemplateSpec{
						Spec: core.PodSpec{
							Containers: []core.Container{container},
							Volumes:    volumes, // Empty slice (not nil) when no secrets
						},
					},
				},
			},
		},
	}
}
