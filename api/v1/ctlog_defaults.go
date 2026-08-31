package v1

import "k8s.io/utils/ptr"

func (s *CTlogSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.PodExtensions.SetDefaults()
	s.Monitoring.SetDefaults()
	s.Ingress.SetDefaults()
	setDefault(&s.MaxCertChainSize, ptr.To(int64(153600)))
	for i := range s.Logs {
		s.Logs[i].SetDefaults()
	}
}

func (c *CTLogConfig) SetDefaults() {
	if c.Signer != nil {
		c.Signer.SetDefaults()
	}
	if c.Prefix == "" {
		c.Prefix = "trusted-artifact-signer"
	}
}

func (s *CTlogSigner) SetDefaults() {
	setDefault(&s.Type, SignerTypeFile)
}
