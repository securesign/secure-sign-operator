package v1

import (
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func (s *RekorSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.Trillian.SetDefaults()
	s.Monitoring.SetDefaults()
	s.ExternalAccess.SetDefaults()
	s.RekorSearchUI.SetDefaults()
	s.Signer.SetDefaults()
	s.Attestations.SetDefaults()
	s.SearchIndex.SetDefaults()
	s.Pvc.SetDefaults()
	s.BackFillRedis.SetDefaults()
	setDefault(&s.MaxRequestBodySize, ptr.To(int64(10485760)))
}

func (s *RekorSearchUI) SetDefaults() {
	s.PodRequirements.SetDefaults()
	setDefault(&s.Enabled, ptr.To(true))
}

func (s *RekorAttestations) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(true))
	setDefault(&s.Url, "file:///var/run/attestations?no_tmp_dir=true")
	if s.MaxSize == nil {
		s.MaxSize = ptr.To(k8sresource.MustParse("100Ki"))
	}
}

func (s *RekorSigner) SetDefaults() {
	setDefault(&s.KMS, "secret")
}

func (s *SearchIndex) SetDefaults() {
	setDefault(&s.Create, ptr.To(true))
}

func (s *BackFillRedis) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(true))
	setDefault(&s.Schedule, "0 0 * * *")
}
