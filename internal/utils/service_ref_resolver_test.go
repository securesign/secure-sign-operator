package utils

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

func TestServiceRefOrAutoload_RefDefaultsNamespace(t *testing.T) {
	g := NewWithT(t)

	trillian := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{Name: "my-trillian", Namespace: "instance-ns"},
	}
	cl := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(trillian).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "my-trillian"},
	}, "instance-ns", instance)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Name).To(Equal("my-trillian"))
	g.Expect(instance.Namespace).To(Equal("instance-ns"))
}

func TestServiceRefOrAutoload_RefNotFound(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	instance := &rhtasv1.Trillian{}
	err := serviceRefOrAutoload(t.Context(), cl, rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{Name: "missing", Namespace: "ns"},
	}, "ns", instance)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("failed to get service")))
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
