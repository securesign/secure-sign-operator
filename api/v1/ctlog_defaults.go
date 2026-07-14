package v1

import "k8s.io/utils/ptr"

func (s *CTlogSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.Monitoring.SetDefaults()
	s.Trillian.SetDefaults()
	setDefault(&s.MaxCertChainSize, ptr.To(int64(153600)))
}
