package v1

func (s *FulcioSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.PodExtensions.SetDefaults()
	s.Monitoring.SetDefaults()
	s.Ingress.SetDefaults()
	s.Signer.SetDefaults()
}

func (s *FulcioSigner) SetDefaults() {
	setDefault(&s.Type, SignerTypeFile)
}
