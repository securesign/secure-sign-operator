package statusurl

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/serviceresolver"
	testaction "github.com/securesign/operator/internal/testing/action"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testIngressName = "test-ingress"
	testInternalURL = "http://internal-service.test.svc:8080/internal/path"
)

func init() {
	serviceresolver.Register(func(*rhtasv1.Fulcio) (string, error) {
		return testInternalURL, nil
	})
}

type testStatusResolver struct {
	enabled bool
	path    string
}

func (r *testStatusResolver) IsEnabled(_ *rhtasv1.Fulcio) bool {
	return r.enabled
}

func (r *testStatusResolver) GetIngressName() string {
	return testIngressName
}

func (r *testStatusResolver) GetIngressPath(_ *rhtasv1.Fulcio) string {
	return r.path
}

func (r *testStatusResolver) GetStatusURL(instance *rhtasv1.Fulcio) string {
	return instance.Status.URL
}

func (r *testStatusResolver) SetStatusURL(instance *rhtasv1.Fulcio, url string) {
	instance.Status.URL = url
}

func TestHandle(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		path        string
		tls         []networkingv1.IngressTLS
		annotations map[string]string
		omitIngress bool
		previousURL string
		expectedURL string
	}{
		{name: "no TLS uses HTTP", enabled: true, expectedURL: "http://example.com"},
		{name: "empty TLS list uses HTTP", enabled: true, tls: []networkingv1.IngressTLS{}, expectedURL: "http://example.com"},
		{name: "empty TLS entry without termination uses HTTP", enabled: true, tls: []networkingv1.IngressTLS{{}}, expectedURL: "http://example.com"},
		{name: "custom TLS certificate without termination uses HTTP", enabled: true, tls: []networkingv1.IngressTLS{{SecretName: "custom-cert"}}, expectedURL: "http://example.com"},
		{name: "annotation uses HTTPS without TLS", enabled: true, annotations: map[string]string{annotations.IngressTermination: "edge"}, expectedURL: "https://example.com"},
		{name: "path without leading slash", enabled: true, path: "active-log", expectedURL: "http://example.com/active-log"},
		{name: "path with leading slash", enabled: true, path: "/api/v1/timestamp", expectedURL: "http://example.com/api/v1/timestamp"},
		{name: "raw path is escaped once", enabled: true, path: "active log/50%", expectedURL: "http://example.com/active%20log/50%25"},
		{name: "disabled ingress needs no ingress object", omitIngress: true, expectedURL: testInternalURL},
		{name: "disabled ingress preserves internal path", omitIngress: true, path: "/external/path", expectedURL: testInternalURL},
		{name: "disabled ingress ignores TLS and termination", tls: []networkingv1.IngressTLS{{}}, annotations: map[string]string{annotations.IngressTermination: "edge"}, expectedURL: testInternalURL},
		{name: "changed URL is updated", enabled: true, tls: []networkingv1.IngressTLS{{}}, annotations: map[string]string{annotations.IngressTermination: "edge"}, previousURL: "http://old.example.com", expectedURL: "https://example.com"},
		{name: "unchanged ingress URL is not written", enabled: true, tls: []networkingv1.IngressTLS{{}}, annotations: map[string]string{annotations.IngressTermination: "edge"}, previousURL: "https://example.com", expectedURL: "https://example.com"},
		{name: "unchanged URL with path is not written", enabled: true, tls: []networkingv1.IngressTLS{{}}, annotations: map[string]string{annotations.IngressTermination: "edge"}, path: "active-log", previousURL: "https://example.com/active-log", expectedURL: "https://example.com/active-log"},
		{name: "unchanged internal URL is not written", omitIngress: true, previousURL: testInternalURL, expectedURL: testInternalURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := &rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: "test"}}
			instance.Status.URL = tt.previousURL
			builder := testaction.FakeClientBuilder().WithStatusSubresource(instance).WithObjects(instance)
			if !tt.omitIngress {
				builder.WithObjects(&networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: testIngressName, Namespace: "test", Annotations: tt.annotations},
					Spec:       networkingv1.IngressSpec{TLS: tt.tls, Rules: []networkingv1.IngressRule{{Host: "example.com"}}},
				})
			}
			cli := builder.Build()
			key := client.ObjectKeyFromObject(instance)
			g.Expect(cli.Get(t.Context(), key, instance)).To(Succeed())
			resourceVersion := instance.ResourceVersion
			a := testaction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](&testStatusResolver{enabled: tt.enabled, path: tt.path}))

			result := a.Handle(t.Context(), instance)
			if tt.previousURL == tt.expectedURL {
				g.Expect(result).To(BeNil())
			} else {
				g.Expect(result).To(Equal(testaction.Return()))
			}
			g.Expect(instance.Status.URL).To(Equal(tt.expectedURL))
			stored := &rhtasv1.Fulcio{}
			g.Expect(cli.Get(t.Context(), key, stored)).To(Succeed())
			g.Expect(stored.Status.URL).To(Equal(tt.expectedURL))
			if tt.previousURL == tt.expectedURL {
				g.Expect(stored.ResourceVersion).To(Equal(resourceVersion))
			}
		})
	}
}
