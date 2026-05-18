package kubernetes

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/securesign/operator/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCalculateHostname(t *testing.T) {
	tests := []struct {
		name     string
		template string
		svcName  string
		ns       string
		expected string
	}{
		{
			name:     "default template produces static .local hostname",
			template: "%[1]s.local",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server.local",
		},
		{
			name:     "namespace-scoped template includes namespace",
			template: "%[1]s.%[2]s.127.0.0.1.nip.io",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server.test-ns.127.0.0.1.nip.io",
		},
		{
			name:     "custom template with different format",
			template: "%[1]s-%[2]s.example.com",
			svcName:  "fulcio-server",
			ns:       "my-namespace",
			expected: "fulcio-server-my-namespace.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := config.IngressHostTemplate
			origOpenshift := config.Openshift
			t.Cleanup(func() {
				config.IngressHostTemplate = original
				config.Openshift = origOpenshift
			})

			config.IngressHostTemplate = tt.template
			config.Openshift = false

			result, err := CalculateHostname(context.Background(), nil, tt.svcName, tt.ns)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCalculateHostname_OpenShift(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1.AddToScheme(scheme))

	tests := []struct {
		name     string
		domain   string
		svcName  string
		ns       string
		expected string
	}{
		{
			name:     "OpenShift hostname includes namespace and cluster domain",
			domain:   "apps.cluster.example.com",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server-test-ns.apps.cluster.example.com",
		},
		{
			name:     "OpenShift hostname with different service and namespace",
			domain:   "apps.ocp.internal",
			svcName:  "fulcio-server",
			ns:       "secure-sign",
			expected: "fulcio-server-secure-sign.apps.ocp.internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origOpenshift := config.Openshift
			t.Cleanup(func() { config.Openshift = origOpenshift })
			config.Openshift = true

			cli := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&configv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec:       configv1.IngressSpec{Domain: tt.domain},
				}).
				Build()

			result, err := CalculateHostname(context.Background(), cli, tt.svcName, tt.ns)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCalculateHostname_OpenShift_MissingIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1.AddToScheme(scheme))

	origOpenshift := config.Openshift
	t.Cleanup(func() { config.Openshift = origOpenshift })
	config.Openshift = true

	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := CalculateHostname(context.Background(), cli, "rekor-server", "test-ns")
	if err == nil {
		t.Fatal("expected error when cluster Ingress is missing, got nil")
	}
}
