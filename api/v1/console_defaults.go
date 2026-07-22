package v1

func (s *ConsoleSpec) SetDefaults() {
	s.UI.SetDefaults()
	s.Api.SetDefaults()
}

func (s *ConsoleUI) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.Ingress.SetDefaults()
}

func (s *ConsoleAPI) SetDefaults() {
	s.PodRequirements.SetDefaults()
}
