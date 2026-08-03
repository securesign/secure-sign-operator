package utils

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/serviceresolver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	serviceresolver.Register(func(obj *rhtasv1.Rekor) (string, error) {
		return fmt.Sprintf("http://rekor-server.%s.svc:3000", obj.Namespace), nil
	})
	serviceresolver.Register(func(obj *rhtasv1.Trillian) (string, error) {
		return fmt.Sprintf("dns:///trillian-logserver.%s.svc:8091", obj.Namespace), nil
	})
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(s)
	return s
}

func TestServiceRefOrAutoload_RefWithNamespace(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "other-ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-rekor", Namespace: "other-ns"},
	}, "default-ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("my-rekor"))
	g.Expect(instance.Namespace).To(Equal("other-ns"))
}

func TestServiceRefOrAutoload_RefEmptyNamespaceFails(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "instance-ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-rekor"},
	}, "instance-ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrGetServiceFailed))
}

func TestServiceRefOrAutoload_RefNotFound(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "missing", Namespace: "ns"},
	}, "ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrGetServiceFailed))
}

func TestServiceRefOrAutoload_AutoloadSingleInstance(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "only-one", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("only-one"))
}

func TestServiceRefOrAutoload_AutoloadNoInstances(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "empty-ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("failed to autodiscovery service")))
}

func TestServiceRefOrAutoload_AutoloadMultipleInstances(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		&rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "ns"}},
		&rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "ns"}},
	).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("found 2 instances")))
}

func TestServiceRefOrAutoload_EmptyRefNameTriggersAutoload(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "discovered", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()

	instance := &rhtasv1.Rekor{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: ""},
	}, "ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("discovered"))
}

func TestResolveInternalServiceUrl(t *testing.T) {
	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()
	// resolved: http://rekor-server.ns.svc:3000
	// url.Parse: Scheme="http" Host="rekor-server.ns.svc:3000" Path=""

	tests := []struct {
		name        string
		url         string
		wantAddress string
	}{
		{
			name:        "full http URL returns as-is",
			url:         "http://custom-host:9090/custom-path",
			wantAddress: "http://custom-host:9090/custom-path",
		},
		{
			name:        "http URL without port",
			url:         "http://custom-host/custom-path",
			wantAddress: "http://custom-host/custom-path",
		},
		{
			name:        "http URL without path",
			url:         "http://custom-host:9090",
			wantAddress: "http://custom-host:9090",
		},
		{
			name:        "empty URL resolves from autodiscovery",
			url:         "",
			wantAddress: "http://rekor-server.ns.svc:3000",
		},
		{
			name:        "schemeless port only merges resolved host and scheme with user port",
			url:         "//:9090",
			wantAddress: "http://rekor-server.ns.svc:9090",
		},
		{
			name:        "schemeless port and path keeps both with resolved host and scheme",
			url:         "//:9090/custom-path",
			wantAddress: "http://rekor-server.ns.svc:9090/custom-path",
		},
		{
			name:        "bare path merges with resolved",
			url:         "custom-path",
			wantAddress: "http://rekor-server.ns.svc:3000/custom-path",
		},
		{
			name:        "absolute path merges with resolved",
			url:         "/custom-path",
			wantAddress: "http://rekor-server.ns.svc:3000/custom-path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			address, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
				URL: tt.url,
				Ref: &rhtasv1.ServiceReferenceRef{Name: "my-rekor", Namespace: "ns"},
			}, "ns", &rhtasv1.Rekor{})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(address).To(Equal(tt.wantAddress))
		})
	}
}

func TestResolveInternalGrpcService(t *testing.T) {
	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()
	// resolved: dns:///trillian-logserver.ns.svc:8091

	tests := []struct {
		name        string
		url         string
		wantAddress string
		wantPort    string
	}{
		{
			name:        "full gRPC URL returns as-is",
			url:         "dns:///custom-host:9090",
			wantAddress: "dns:///custom-host",
			wantPort:    "9090",
		},
		{
			name:        "gRPC URL without port",
			url:         "dns:///custom-host",
			wantAddress: "dns:///custom-host",
			wantPort:    "",
		},
		{
			name:        "empty URL resolves from autodiscovery",
			url:         "",
			wantAddress: "dns:///trillian-logserver.ns.svc",
			wantPort:    "8091",
		},
		{
			name:        "schemeless empty with port overrides resolved port",
			url:         ":9090",
			wantAddress: "dns:///trillian-logserver.ns.svc",
			wantPort:    "9090",
		},
		{
			name:        "schemeless empty with port overrides resolved port",
			url:         "//:9090",
			wantAddress: "dns:///trillian-logserver.ns.svc",
			wantPort:    "9090",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			address, port, err := ResolveInternalGrpcService(t.Context(), cl, rhtasv1.ServiceReference{
				URL: tt.url,
				Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian", Namespace: "ns"},
			}, "ns", &rhtasv1.Trillian{})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(address).To(Equal(tt.wantAddress))
			g.Expect(port).To(Equal(tt.wantPort))
		})
	}
}

func TestResolveExternalServiceUrl_URLTakesPrecedence(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(rekor).Build()

	u, err := ResolveExternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
		URL: "https://external.example.com",
	}, "ns", &rhtasv1.Rekor{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("https://external.example.com"))
}

func TestResolveExternalServiceUrl_SchemelessMergesPortAndPath(t *testing.T) {
	setup := func(t *testing.T) client.Client {
		t.Helper()
		rekor := &rhtasv1.Rekor{
			ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "ns"},
		}
		cl := fake.NewClientBuilder().WithScheme(testScheme()).WithStatusSubresource(rekor).WithObjects(rekor).Build()
		rekor.Status.Url = "https://rekor.apps.cluster.example.com/default-path"
		rekor.Status.Conditions = []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", LastTransitionTime: metav1.Now()},
		}
		NewWithT(t).Expect(cl.Status().Update(t.Context(), rekor)).To(Succeed())
		return cl
	}

	t.Run("path only merges with resolved host", func(t *testing.T) {
		g := NewWithT(t)
		u, err := ResolveExternalServiceUrl(t.Context(), setup(t),
			rhtasv1.ServiceReference{URL: "///custom-prefix"}, "ns", &rhtasv1.Rekor{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(u).To(Equal("https://rekor.apps.cluster.example.com/custom-prefix"))
	})

	t.Run("port and path merge with resolved host", func(t *testing.T) {
		g := NewWithT(t)
		u, err := ResolveExternalServiceUrl(t.Context(), setup(t),
			rhtasv1.ServiceReference{URL: "//:8080/custom-prefix"}, "ns", &rhtasv1.Rekor{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(u).To(Equal("https://rekor.apps.cluster.example.com:8080/custom-prefix"))
	})

	t.Run("empty URL uses resolved as-is", func(t *testing.T) {
		g := NewWithT(t)
		u, err := ResolveExternalServiceUrl(t.Context(), setup(t),
			rhtasv1.ServiceReference{}, "ns", &rhtasv1.Rekor{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(u).To(Equal("https://rekor.apps.cluster.example.com/default-path"))
	})
}

func TestResolveExternalServiceUrl_FromStatus(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithStatusSubresource(rekor).WithObjects(rekor).Build()

	rekor.Status.Url = "http://rekor.internal.svc"
	rekor.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", LastTransitionTime: metav1.Now()},
	}
	g.Expect(cl.Status().Update(t.Context(), rekor)).To(Succeed())

	u, err := ResolveExternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-rekor", Namespace: "ns"},
	}, "ns", &rhtasv1.Rekor{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("http://rekor.internal.svc"))
}

func TestResolveExternalServiceUrl_NotReady(t *testing.T) {
	g := NewWithT(t)

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rekor", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithStatusSubresource(rekor).WithObjects(rekor).Build()

	rekor.Status.Url = "http://rekor.internal.svc"
	rekor.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Creating", LastTransitionTime: metav1.Now()},
	}
	g.Expect(cl.Status().Update(t.Context(), rekor)).To(Succeed())

	_, err := ResolveExternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-rekor", Namespace: "ns"},
	}, "ns", &rhtasv1.Rekor{})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrServiceNotReady))
}
