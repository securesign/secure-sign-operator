package utils

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/serviceresolver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	serviceresolver.Register(func(obj *rhtasv1.Trillian) (string, error) {
		return fmt.Sprintf("dns:///trillian-logserver.%s.svc:8091", obj.Namespace), nil
	})
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(s)
	return s
}

func TestResolveInternalServiceUrl_URL(t *testing.T) {
	g := NewWithT(t)

	u, err := ResolveInternalServiceUrl(t.Context(), nil, rhtasv1.ServiceReference{
		URL: "https://rekor.example.com",
	}, "default", &rhtasv1.Trillian{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("https://rekor.example.com"))
}

func TestResolveInternalServiceUrl_Ref(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	u, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian", Namespace: "ns"},
	}, "ns", &rhtasv1.Trillian{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("dns:///trillian-logserver.ns.svc:8091"))
}

func TestResolveInternalServiceUrl_RefNotFound(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "missing", Namespace: "default"},
	}, "default", &rhtasv1.Trillian{})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("failed to get service")))
}

func TestResolveInternalServiceUrl_Autodiscovery(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	u, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", &rhtasv1.Trillian{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("dns:///trillian-logserver.ns.svc:8091"))
}

func TestResolveInternalServiceUrl_AutodiscoveryEmpty(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{}, "empty-ns", &rhtasv1.Trillian{})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("no")))
}

func TestResolveInternalServiceUrl_AutodiscoveryMultiple(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		&rhtasv1.Trillian{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "ns"}},
		&rhtasv1.Trillian{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "ns"}},
	).Build()

	_, err := ResolveInternalServiceUrl(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", &rhtasv1.Trillian{})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("found 2 instances")))
}

func TestResolveInternalServiceUrl_URLBareHostPort(t *testing.T) {
	g := NewWithT(t)

	u, err := ResolveInternalServiceUrl(t.Context(), nil, rhtasv1.ServiceReference{
		URL: "trillian.default.svc:8091",
	}, "default", &rhtasv1.Trillian{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("trillian.default.svc:8091"))
}

func TestResolveInternalServiceUrl_URLTakesPrecedence(t *testing.T) {
	g := NewWithT(t)

	u, err := ResolveInternalServiceUrl(t.Context(), nil, rhtasv1.ServiceReference{
		URL: "https://external.example.com",
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian", Namespace: "ns"},
	}, "ns", &rhtasv1.Trillian{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(u).To(Equal("https://external.example.com"))
}
