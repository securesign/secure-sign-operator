package v1

import (
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func (s *TufSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.ExternalAccess.SetDefaults()
	setDefault(&s.Port, int32(80))
	setDefaultSlice(&s.Keys, []TufKey{
		{Name: TufKeyRekor},
		{Name: TufKeyCTFE},
		{Name: TufKeyFulcio},
		{Name: TufKeyTSA},
	})
	if s.RootKeySecretRef == nil {
		s.RootKeySecretRef = &LocalObjectReference{Name: "tuf-root-keys"}
	}
	if s.Pvc.Size == nil {
		s.Pvc.Size = ptr.To(k8sresource.MustParse("100Mi"))
	}
	s.Pvc.SetDefaults()
	s.Ctlog.SetDefaults()
}
