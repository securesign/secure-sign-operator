package server

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestRekorServerMonitoringConfig_IsEnabled(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := rekorServerMonitoringConfig{}

	g.Expect(cfg.IsEnabled(&rhtasv1.Rekor{
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
				},
			},
		},
	})).To(BeTrue())

	g.Expect(cfg.IsEnabled(&rhtasv1.Rekor{
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)},
				},
			},
		},
	})).To(BeFalse())
}

func TestRekorServerMonitoringConfig_TLS(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := rekorServerMonitoringConfig{}.TLS(&rhtasv1.Rekor{})
	g.Expect(tls.CertRef).To(BeNil())
}
