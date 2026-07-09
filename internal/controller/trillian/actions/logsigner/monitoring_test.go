package logsigner

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestLogsignerMonitoringConfig_IsEnabled(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := logsignerMonitoringConfig{}

	g.Expect(cfg.IsEnabled(&rhtasv1.Trillian{
		Spec: rhtasv1.TrillianSpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(true)},
			},
		},
	})).To(BeTrue())

	g.Expect(cfg.IsEnabled(&rhtasv1.Trillian{
		Spec: rhtasv1.TrillianSpec{
			Monitoring: rhtasv1.MonitoringConfig{
				ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)},
			},
		},
	})).To(BeFalse())
}

func TestLogsignerMonitoringConfig_TLS_WithCert(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := logsignerMonitoringConfig{}

	instance := &rhtasv1.Trillian{
		Status: rhtasv1.TrillianStatus{
			LogSigner: rhtasv1.TrillianServiceStatus{
				TLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-tls"},
						Key:                  "ca.crt",
					},
				},
			},
		},
	}

	tls := cfg.TLS(instance)
	g.Expect(tls.CertRef).ToNot(BeNil())
	g.Expect(tls.CertRef.Name).To(Equal("signer-tls"))
}

func TestLogsignerMonitoringConfig_TLS_WithoutCert(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := logsignerMonitoringConfig{}.TLS(&rhtasv1.Trillian{})
	g.Expect(tls.CertRef).To(BeNil())
}
