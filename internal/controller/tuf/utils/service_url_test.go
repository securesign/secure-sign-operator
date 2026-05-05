package utils

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var scheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	return scheme
}()

func TestResolveServiceAddress_UserSpecifiedAddress(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			Rekor: v1alpha1.RekorService{
				Address: "http://rekor.fakeserver.com",
			},
			Ctlog: v1alpha1.CtlogService{
				Address: "http://ctlog.fakeserver.com",
			},
			Fulcio: v1alpha1.FulcioService{
				Address: "http://fulcio.fakeserver.com",
			},
			Tsa: v1alpha1.TsaService{
				Address: "http://tsa.fakeserver.com",
			},
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "ctfe.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
				{
					Name: "tsa.certchain.pem",
				},
			},
		},
	}
	err := ResolveServiceAddress(t.Context(), c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Spec.Rekor.Address).To(Equal("http://rekor.fakeserver.com"))
	g.Expect(instance.Spec.Ctlog.Address).To(Equal("http://ctlog.fakeserver.com"))
	g.Expect(instance.Spec.Fulcio.Address).To(Equal("http://fulcio.fakeserver.com"))
	// do not append the timestamp path to the user-provided address
	g.Expect(instance.Spec.Tsa.Address).To(Equal("http://tsa.fakeserver.com"))
}

func TestResolveServiceAddress_InternalServiceLoading(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.Rekor{}).Build()

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
			},
		},
	}
	err := ResolveServiceAddress(t.Context(), c, instance)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(And(ContainSubstring("no items found in"), ContainSubstring("Rekor"))))

	rekor := &v1alpha1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
	}
	g.Expect(c.Create(t.Context(), rekor)).To(Succeed())

	err = ResolveServiceAddress(t.Context(), c, instance)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(And(ContainSubstring("service is not ready"), ContainSubstring("test"))))

	rekor.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		},
	}
	rekor.Status.Url = "http://rekor.fakeserver.com"
	g.Expect(c.Status().Update(t.Context(), rekor)).To(Succeed())
	err = ResolveServiceAddress(t.Context(), c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Spec.Rekor.Address).To(Equal("http://rekor.fakeserver.com"))
}
