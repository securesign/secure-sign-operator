package monitor

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestRekorMonitorConfig_IsEnabled_RequiresBothTLogAndServiceMonitor(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := rekorMonitorMonitoringConfig{}

	g.Expect(cfg.IsEnabled(&rhtasv1.Rekor{
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
				},
				TLog: rhtasv1.TlogMonitoring{Enabled: ptr.To(true)},
			},
		},
	})).To(BeTrue(), "both enabled = true")

	g.Expect(cfg.IsEnabled(&rhtasv1.Rekor{
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
				},
				TLog: rhtasv1.TlogMonitoring{Enabled: ptr.To(false)},
			},
		},
	})).To(BeFalse(), "TLog disabled = no ServiceMonitor for monitor pods")

	g.Expect(cfg.IsEnabled(&rhtasv1.Rekor{
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)},
				},
				TLog: rhtasv1.TlogMonitoring{Enabled: ptr.To(true)},
			},
		},
	})).To(BeFalse(), "ServiceMonitor disabled = no ServiceMonitor")
}

func TestRekorMonitorConfig_TLS(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := rekorMonitorMonitoringConfig{}.TLS(&rhtasv1.Rekor{})
	g.Expect(tls.CertRef).To(BeNil())
}
