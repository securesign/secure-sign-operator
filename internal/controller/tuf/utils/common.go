package utils

import (
	"github.com/securesign/operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
)

func secretsVolumeProjection(keys []v1alpha1.TufKey) *core.ProjectedVolumeSource {

	projections := make([]core.VolumeProjection, 0)

	for _, key := range keys {
		p := core.VolumeProjection{Secret: selectorToProjection(key.SecretRef, key.Name)}
		projections = append(projections, p)
	}

	return &core.ProjectedVolumeSource{
		Sources: projections,
	}
}

func selectorToProjection(secret *v1alpha1.SecretKeySelector, path string) *core.SecretProjection {
	return &core.SecretProjection{
		LocalObjectReference: core.LocalObjectReference{
			Name: secret.Name,
		},
		Items: []core.KeyToPath{
			{
				Key:  secret.Key,
				Path: path,
			},
		},
	}
}
