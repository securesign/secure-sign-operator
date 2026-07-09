package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestFulcioMonitoringConfig_IsEnabled(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := fulcioMonitoringConfig{}

	g.Expect(cfg.IsEnabled(&rhtasv1.Fulcio{
		Spec: rhtasv1.FulcioSpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
			},
		},
	})).To(BeTrue())

	g.Expect(cfg.IsEnabled(&rhtasv1.Fulcio{
		Spec: rhtasv1.FulcioSpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)},
			},
		},
	})).To(BeFalse())
}

func TestFulcioMonitoringConfig_TLS(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := fulcioMonitoringConfig{}.TLS(&rhtasv1.Fulcio{})
	g.Expect(tls.CertRef).To(BeNil())
}
