package utils

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

var ErrorResolveServiceUrl = fmt.Errorf("failed to resolve service url")

func secretsVolumeProjection(keys []rhtasv1.TufKeyStatus) *corev1ac.ProjectedVolumeSourceApplyConfiguration {

	projections := make([]*corev1ac.VolumeProjectionApplyConfiguration, 0, len(keys))

	for _, key := range keys {
		p := corev1ac.VolumeProjection().WithSecret(
			corev1ac.SecretProjection().WithName(key.SecretRef.Name).WithItems(corev1ac.KeyToPath().WithKey(key.SecretRef.Key).WithPath(key.Name)),
		)
		projections = append(projections, p)
	}

	return corev1ac.ProjectedVolumeSource().WithSources(projections...)
}
