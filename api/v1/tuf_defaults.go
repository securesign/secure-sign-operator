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
	s.Pvc.SetDefaults()
	s.Ctlog.SetDefaults()
}

func (s *TufPvc) SetDefaults() {
	if s.Size == nil {
		s.Size = ptr.To(k8sresource.MustParse("100Mi"))
	}
	setDefault(&s.Retain, ptr.To(true))
	setDefaultSlice(&s.AccessModes, []PersistentVolumeAccessMode{"ReadWriteOnce"})
}
