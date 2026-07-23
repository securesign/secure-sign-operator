package utils

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
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
	utilruntime.Must(rhtasv1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	return scheme
}()

func TestResolveServiceAddress_UserSpecifiedURL(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	instance := &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: rhtasv1.TufSpec{
			Rekor:  rhtasv1.ServiceReference{URL: "http://rekor.fakeserver.com"},
			Ctlog:  rhtasv1.ServiceReference{URL: "http://ctlog.fakeserver.com"},
			Fulcio: rhtasv1.ServiceReference{URL: "http://fulcio.fakeserver.com"},
			Tsa:    rhtasv1.ServiceReference{URL: "http://tsa.fakeserver.com"},
			Keys: []rhtasv1.TufKey{
				{Name: rhtasv1.TufKeyRekor},
				{Name: rhtasv1.TufKeyCTFE},
				{Name: rhtasv1.TufKeyFulcio},
				{Name: rhtasv1.TufKeyTSA},
			},
		},
	}

	for _, key := range instance.Spec.Keys {
		result, err := resolveServiceAddress(t.Context(), c, instance, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Address).ToNot(BeEmpty())
	}

	result, err := resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyRekor)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://rekor.fakeserver.com"))

	result, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyTSA)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://tsa.fakeserver.com"))
}

func TestResolveServiceAddress_Autoload(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rhtasv1.Rekor{}).Build()

	instance := &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: rhtasv1.TufSpec{
			Keys: []rhtasv1.TufKey{
				{Name: rhtasv1.TufKeyRekor},
			},
		},
	}

	_, err := resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyRekor)
	g.Expect(err).To(HaveOccurred())

	rekor := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
	}
	g.Expect(c.Create(t.Context(), rekor)).To(Succeed())

	_, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyRekor)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("service url is empty")))

	rekor.Status.Url = "http://rekor.internal.svc"
	g.Expect(c.Status().Update(t.Context(), rekor)).To(Succeed())

	result, err := resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyRekor)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://rekor.internal.svc"))
}
