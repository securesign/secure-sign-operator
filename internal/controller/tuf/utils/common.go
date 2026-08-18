package utils

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	core "k8s.io/api/core/v1"
)

var ErrorResolveServiceUrl = fmt.Errorf("failed to resolve service url")

func secretsVolumeProjection(keys []rhtasv1.TufKeyStatus) *core.ProjectedVolumeSource {

	projections := make([]core.VolumeProjection, 0, len(keys))

	for _, key := range keys {
		p := core.VolumeProjection{Secret: selectorToProjection(key.SecretRef, key.Name)}
		projections = append(projections, p)
	}

	return &core.ProjectedVolumeSource{
		Sources: projections,
	}
}

func selectorToProjection(secret *rhtasv1.SecretKeySelector, path string) *core.SecretProjection {
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
