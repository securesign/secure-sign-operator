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

func TestResolveServiceAddress_URL(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	instance := &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: rhtasv1.TufSpec{
			Rekor: rhtasv1.ServiceReference{URL: "http://rekor.fakeserver.com"},
			Ctlog: rhtasv1.ServiceReference{URL: "http://ctlog.fakeserver.com"},
			Fulcio: rhtasv1.ServiceRefWithOIDC{
				ServiceReference: rhtasv1.ServiceReference{URL: "http://fulcio.fakeserver.com"},
				OIDCIssuers:      []string{"https://accounts.google.com", "https://login.microsoftonline.com"},
			},
			Tsa: rhtasv1.ServiceReference{URL: "http://tsa.fakeserver.com"},
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
	g.Expect(result.OIDCIssuers).To(BeEmpty())

	result, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyCTFE)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://ctlog.fakeserver.com"))
	g.Expect(result.OIDCIssuers).To(BeEmpty())

	result, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyFulcio)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://fulcio.fakeserver.com"))
	g.Expect(result.OIDCIssuers).To(Equal([]string{"https://accounts.google.com", "https://login.microsoftonline.com"}))

	result, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyTSA)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("http://tsa.fakeserver.com"))
	g.Expect(result.OIDCIssuers).To(BeEmpty())
}

func TestResolveServiceAddress_Autodiscovery(t *testing.T) {
	g := NewWithT(t)

	fulcio := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: t.Name()},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(fulcio).Build()

	instance := &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: rhtasv1.TufSpec{
			Keys: []rhtasv1.TufKey{
				{Name: rhtasv1.TufKeyFulcio},
			},
		},
	}

	_, err := resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyFulcio)
	g.Expect(err).To(HaveOccurred())

	g.Expect(c.Create(t.Context(), fulcio)).To(Succeed())

	_, err = resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyFulcio)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("service is not ready")))

	fulcio.Spec.Config.OIDCIssuers = []rhtasv1.OIDCIssuer{
		{IssuerURL: "https://keycloak.example.com/realms/trusted", Issuer: "keycloak", ClientID: "sigstore"},
		{Issuer: "https://github.com/login/oauth", ClientID: "sigstore"},
	}
	g.Expect(c.Update(t.Context(), fulcio)).To(Succeed())

	fulcio.Status.Url = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", LastTransitionTime: metav1.Now()},
	}
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	result, err := resolveServiceAddress(t.Context(), c, instance, rhtasv1.TufKeyFulcio)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Address).To(Equal("https://fulcio.internal.svc"))
	g.Expect(result.OIDCIssuers).To(Equal([]string{"https://keycloak.example.com/realms/trusted", "https://github.com/login/oauth"}))
}
