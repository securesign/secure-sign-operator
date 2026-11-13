package traits

import (
	"testing"

	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
)

func TestIngressHost_Hostname(t *testing.T) {
	originalOpenShift, originalTemplate := config.Openshift, config.IngressHostTemplate
	t.Cleanup(func() {
		config.Openshift, config.IngressHostTemplate = originalOpenShift, originalTemplate
	})
	config.IngressHostTemplate = "%[1]s.%[2]s.example.com"

	tests := []struct {
		name         string
		openShift    bool
		host         string
		expectedHost string
	}{
		{name: "explicit hostname", host: "custom.example.com", expectedHost: "custom.example.com"},
		{name: "Kubernetes template", expectedHost: "fulcio-server.test.example.com"},
		{name: "OpenShift domain", openShift: true, expectedHost: "fulcio-server-test.apps.example.com"},
		{name: "explicit hostname without cluster config", openShift: true, host: "custom.example.com", expectedHost: "custom.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			config.Openshift = tt.openShift
			c := testAction.FakeClientBuilder().Build()
			g.Expect(configv1.AddToScheme(c.Scheme())).To(gomega.Succeed())
			if tt.openShift {
				g.Expect(c.Create(t.Context(), &configv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec:       configv1.IngressSpec{Domain: "apps.example.com"},
				})).To(gomega.Succeed())
			}
			svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "fulcio-server", Namespace: "test"}}
			conf := rhtasv1.Ingress{Host: tt.host}
			ingress, err := Apply(networkingv1ac.Ingress(svc.Name, svc.Namespace),
				WithRule(
					IngressHost(t.Context(), c, conf, svc),
					IngressPath(svc, "http", "/"),
				))
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(ingress.Spec.Rules).To(gomega.HaveLen(1))
			g.Expect(ingress.Spec.Rules[0].Host).To(gomega.Equal(&tt.expectedHost))
		})
	}
}

func TestWithRule_MultiplePaths(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)
	api := corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}}
	ui := corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "ui", Namespace: "default"}}

	ingress, err := Apply(networkingv1ac.Ingress("test", "default"),
		IngressTLSWithTermination(IngressTerminationEdge),
		WithRule(
			IngressHost(t.Context(), nil, rhtasv1.Ingress{Host: "app.example.com"}, api),
			nil,
			IngressPath(api, "http", "/api"),
			IngressPath(ui, "web", "/"),
		),
		WithRule(
			IngressHost(t.Context(), nil, rhtasv1.Ingress{Host: "admin.example.com"}, api),
			IngressPath(api, "http", "/"),
		),
	)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(ingress.Spec.TLS).To(gomega.BeEmpty())
	g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.IngressTermination, IngressTerminationEdge))
	g.Expect(ingress.Spec.Rules).To(gomega.HaveLen(2))
	g.Expect(*ingress.Spec.Rules[0].Host).To(gomega.Equal("app.example.com"))
	g.Expect(*ingress.Spec.Rules[1].Host).To(gomega.Equal("admin.example.com"))
	paths := ingress.Spec.Rules[0].HTTP.Paths
	g.Expect(paths).To(gomega.HaveLen(2))
	g.Expect(*paths[0].Path).To(gomega.Equal("/api"))
	g.Expect(*paths[0].Backend.Service.Name).To(gomega.Equal("api"))
	g.Expect(*paths[0].Backend.Service.Port.Name).To(gomega.Equal("http"))
	g.Expect(*paths[0].PathType).To(gomega.Equal(networkingv1.PathTypePrefix))
	g.Expect(*paths[1].Path).To(gomega.Equal("/"))
	g.Expect(*paths[1].Backend.Service.Name).To(gomega.Equal("ui"))
	g.Expect(*paths[1].Backend.Service.Port.Name).To(gomega.Equal("web"))
	g.Expect(ingress.Spec.Rules[1].HTTP.Paths).To(gomega.HaveLen(1))
}
