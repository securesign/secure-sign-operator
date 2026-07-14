package v1

import "k8s.io/utils/ptr"

func (s *TimestampAuthoritySpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.Monitoring.SetDefaults()
	s.ExternalAccess.SetDefaults()
	s.NTPMonitoring.SetDefaults()
	setDefault(&s.MaxRequestBodySize, ptr.To(int64(1048576)))
}

func (s *NTPMonitoring) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(true))
}
