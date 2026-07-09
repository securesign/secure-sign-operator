package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestTSAMonitoringConfig_IsEnabled(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := tsaMonitoringConfig{}

	g.Expect(cfg.IsEnabled(&rhtasv1.TimestampAuthority{
		Spec: rhtasv1.TimestampAuthoritySpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
			},
		},
	})).To(BeTrue())

	g.Expect(cfg.IsEnabled(&rhtasv1.TimestampAuthority{
		Spec: rhtasv1.TimestampAuthoritySpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)},
			},
		},
	})).To(BeFalse())
}

func TestTSAMonitoringConfig_TLS(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := tsaMonitoringConfig{}.TLS(&rhtasv1.TimestampAuthority{})
	g.Expect(tls.CertRef).To(BeNil())
}
