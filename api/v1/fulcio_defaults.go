package v1

func (s *FulcioSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.Ctlog.SetDefaults()
	s.Monitoring.SetDefaults()
	s.ExternalAccess.SetDefaults()
}
