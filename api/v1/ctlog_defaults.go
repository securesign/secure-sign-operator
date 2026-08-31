package v1

import "k8s.io/utils/ptr"

func (s *CTlogSpec) SetDefaults() {
	s.PodRequirements.SetDefaults()
	s.PodExtensions.SetDefaults()
	s.Monitoring.SetDefaults()
	s.Ingress.SetDefaults()
	setDefault(&s.MaxCertChainSize, ptr.To(int64(153600)))
	if len(s.Logs) == 0 {
		s.Logs = []CTLogConfig{
			{
				Prefix: "trusted-artifact-signer",
				Active: ptr.To(true),
				Signer: &CTlogSigner{Type: SignerTypeFile},
			},
		}
	}
	for i := range s.Logs {
		s.Logs[i].SetDefaults()
	}
}

func (c *CTLogConfig) SetDefaults() {
	if c.Signer != nil {
		c.Signer.SetDefaults()
	}
}

func (s *CTlogSigner) SetDefaults() {
	setDefault(&s.Type, SignerTypeFile)
}
