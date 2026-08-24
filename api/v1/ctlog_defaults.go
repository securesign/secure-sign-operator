package v1

import "k8s.io/utils/ptr"

func (s *CTlogSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.PodExtensions.SetDefaults()
	s.Monitoring.SetDefaults()
	s.Ingress.SetDefaults()
	s.Signer.SetDefaults()
	setDefault(&s.Prefix, "trusted-artifact-signer")
	setDefault(&s.MaxCertChainSize, ptr.To(int64(153600)))
}

func (s *CTlogSigner) SetDefaults() {
	setDefault(&s.Type, CTlogSignerTypeFile)
}
