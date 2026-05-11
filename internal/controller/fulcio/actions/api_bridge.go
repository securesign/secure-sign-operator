package actions

import (
	"maps"

	rhtasv1alpha1api "github.com/securesign/operator/api/v1alpha1"
	rhtasv1beta1api "github.com/securesign/operator/api/v1beta1"
)

func toAlphaExternalAccess(e rhtasv1beta1api.ExternalAccess) rhtasv1alpha1api.ExternalAccess {
	return rhtasv1alpha1api.ExternalAccess{
		Enabled:             e.Enabled,
		Host:                e.Host,
		RouteSelectorLabels: maps.Clone(e.RouteSelectorLabels),
	}
}

func toAlphaPodRequirements(p rhtasv1beta1api.PodRequirements) rhtasv1alpha1api.PodRequirements {
	return rhtasv1alpha1api.PodRequirements{
		Replicas:    p.Replicas,
		Affinity:    p.Affinity,
		Resources:   p.Resources,
		Tolerations: p.Tolerations,
	}
}

func toAlphaLocalObjectReference(lor *rhtasv1beta1api.LocalObjectReference) *rhtasv1alpha1api.LocalObjectReference {
	if lor == nil {
		return nil
	}
	return &rhtasv1alpha1api.LocalObjectReference{Name: lor.Name}
}

func toAlphaSecretKeySelector(s *rhtasv1beta1api.SecretKeySelector) *rhtasv1alpha1api.SecretKeySelector {
	if s == nil {
		return nil
	}
	return &rhtasv1alpha1api.SecretKeySelector{
		LocalObjectReference: rhtasv1alpha1api.LocalObjectReference{Name: s.Name},
		Key:                  s.Key,
	}
}

// fulcioTLSBridge adapts *v1beta1.Fulcio for utils that still expect v1alpha1 LocalObjectReference.
type fulcioTLSBridge struct {
	*rhtasv1beta1api.Fulcio
}

func newFulcioTLSBridge(f *rhtasv1beta1api.Fulcio) *fulcioTLSBridge {
	return &fulcioTLSBridge{Fulcio: f}
}

func (b *fulcioTLSBridge) GetTrustedCA() *rhtasv1alpha1api.LocalObjectReference {
	return toAlphaLocalObjectReference(b.Fulcio.GetTrustedCA())
}
