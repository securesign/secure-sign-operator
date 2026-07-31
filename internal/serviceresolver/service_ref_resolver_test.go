package serviceresolver

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	Register(func(obj *rhtasv1.Trillian) (string, error) {
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

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "other-ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian", Namespace: "other-ns"},
	}, "default-ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("my-trillian"))
	g.Expect(instance.Namespace).To(Equal("other-ns"))
}

func TestServiceRefOrAutoload_RefEmptyNamespaceFails(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "instance-ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian"},
	}, "instance-ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrGetServiceFailed))
}

func TestServiceRefOrAutoload_RefNotFound(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "missing", Namespace: "ns"},
	}, "ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrGetServiceFailed))
}

func TestServiceRefOrAutoload_AutoloadSingleInstance(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "only-one", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("only-one"))
}

func TestServiceRefOrAutoload_AutoloadNoInstances(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "empty-ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("failed to autodiscovery service")))
}

func TestServiceRefOrAutoload_AutoloadMultipleInstances(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		&rhtasv1.Trillian{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "ns"}},
		&rhtasv1.Trillian{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "ns"}},
	).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{}, "ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("found 2 instances")))
}

func TestServiceRefOrAutoload_EmptyRefNameTriggersAutoload(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "discovered", Namespace: "ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: ""},
	}, "ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("discovered"))
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
