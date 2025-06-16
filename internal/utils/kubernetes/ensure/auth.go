package ensure

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

const (
	authVolumeName = "auth"
	AuthMountPath  = constants.SecretMountPath + "/auth"
)

func Auth(containerName string, auth *v1alpha1.Auth) func(spec *core.PodSpec) error {
	return func(templateSpec *core.PodSpec) error {
		if auth != nil {
			container := kubernetes.FindContainerByNameOrCreate(templateSpec, containerName)
			for _, env := range auth.Env {
				e := kubernetes.FindEnvByNameOrCreate(container, env.Name)
				if !equality.Semantic.DeepEqual(env, e) {
					env.DeepCopyInto(e)
				}
			}
			authProjected := kubernetes.FindVolumeByNameOrCreate(templateSpec, authVolumeName)
			if authProjected == nil {
				authProjected.Projected = &core.ProjectedVolumeSource{}
			}

			for _, secret := range auth.SecretMount {
				findSecretProjectedVolumeByNameOrCreate(authProjected.Projected, secret.Name)
			}

			vm := kubernetes.FindVolumeMountByNameOrCreate(container, authVolumeName)
			vm.MountPath = AuthMountPath
			vm.ReadOnly = true
		}
		return nil
	}
}

func findSecretProjectedVolumeByNameOrCreate(source *core.ProjectedVolumeSource, secretName string) *core.SecretProjection {
	for i, v := range source.Sources {
		if v.Secret != nil && v.Secret.Name == secretName {
			return source.Sources[i].Secret
		}
	}
	source.Sources = append(source.Sources, core.VolumeProjection{
		Secret: &core.SecretProjection{LocalObjectReference: core.LocalObjectReference{Name: secretName}},
	})
	return source.Sources[len(source.Sources)-1].Secret
}
