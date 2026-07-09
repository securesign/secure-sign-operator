package logserver

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestLogserverMonitoringConfig_IsEnabled(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := logserverMonitoringConfig{}

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

func TestLogserverMonitoringConfig_TLS_WithCert(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	cfg := logserverMonitoringConfig{}

	instance := &rhtasv1.Trillian{
		Status: rhtasv1.TrillianStatus{
			LogServer: rhtasv1.TrillianServiceStatus{
				TLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tls-secret"},
						Key:                  "ca.crt",
					},
				},
			},
		},
	}

	tls := cfg.TLS(instance)
	g.Expect(tls.CertRef).ToNot(BeNil())
	g.Expect(tls.CertRef.Name).To(Equal("tls-secret"))
	g.Expect(tls.CertRef.Key).To(Equal("ca.crt"))
}

func TestLogserverMonitoringConfig_TLS_WithoutCert(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	tls := logserverMonitoringConfig{}.TLS(&rhtasv1.Trillian{})
	g.Expect(tls.CertRef).To(BeNil())
}
