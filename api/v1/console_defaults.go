package v1

func (s *ConsoleSpec) SetDefaults() {
	s.UI.PodRequirements.SetDefaults()
	s.UI.ExternalAccess.SetDefaults()
	s.Api.PodRequirements.SetDefaults()
}
