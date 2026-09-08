package trustroot

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testPEM = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZFt6NEqMxaeU76lnlYzFUNjFQGHq
NF46BPCTlP/FgfMZjN608cDXf3LM5hTbvNyCEabE+4MbOcEMXhDQUlYFvA==
-----END PUBLIC KEY-----
`

var scheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1.AddToScheme(scheme))
	return scheme
}()

func readyCondition() metav1.Condition {
	return metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}
}

func TestResolve_ExplicitSecretRefAndURL(t *testing.T) {
	g := NewWithT(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "keys", Namespace: t.Name()},
		Data:       map[string][]byte{"rekor.pub": []byte(testPEM)},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{URL: "http://rekor.fakeserver.com"},
		SecretRef:        &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"}, Key: "rekor.pub"},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), Rekor, binding)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Address).To(Equal("http://rekor.fakeserver.com"))
	g.Expect(resolved.Material).To(Equal([]byte(testPEM)))
}

func TestResolve_RefBased(t *testing.T) {
	g := NewWithT(t)
	rekor := &rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: t.Name()}}
	rekor.Status.PublicKey = testPEM
	rekor.Status.Url = "https://rekor.internal.svc"
	rekor.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rekor).WithStatusSubresource(rekor).Build()
	g.Expect(c.Status().Update(t.Context(), rekor)).To(Succeed())

	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "rekor", Namespace: t.Name()}},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), Rekor, binding)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Address).To(Equal("https://rekor.internal.svc"))
	g.Expect(resolved.Material).To(Equal([]byte(testPEM)))
}

func TestResolve_Autodiscovery(t *testing.T) {
	g := NewWithT(t)
	ctlog := &rhtasv1.CTlog{ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: t.Name()}}
	ctlog.Status.Logs = []rhtasv1.CTlogLogStatus{
		{
			Prefix:    "test-log",
			Active:    true,
			PublicKey: testPEM,
		},
	}
	ctlog.Status.Url = "https://ctlog.internal.svc"
	ctlog.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ctlog).WithStatusSubresource(ctlog).Build()
	g.Expect(c.Status().Update(t.Context(), ctlog)).To(Succeed())

	resolved, err := Resolve(t.Context(), c, t.Name(), CTFE, rhtasv1.TrustRootBinding{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Address).To(Equal("https://ctlog.internal.svc"))
	g.Expect(resolved.Material).To(Equal([]byte(testPEM)))
}

func TestResolve_ExplicitURLAddress_MaterialFromStatus_NotReadyRegression(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()}}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.Url = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{URL: "http://fulcio.fakeserver.com"},
	}

	_, err := Resolve(t.Context(), c, t.Name(), Fulcio, binding)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ErrComponentNotReady))
}

func TestResolveFulcio_ExplicitOIDCIssuersWin(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()},
		Spec: rhtasv1.FulcioSpec{
			Config: rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{{IssuerURL: "https://from-ref.example.com", ClientID: "sigstore"}}},
		},
	}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.Url = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{
		TrustRootBinding: rhtasv1.TrustRootBinding{
			ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "fulcio", Namespace: t.Name()}},
		},
		OIDCIssuers: []string{"https://explicit.example.com"},
	}

	resolved, err := ResolveFulcio(t.Context(), c, t.Name(), binding)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.OIDCIssuers).To(Equal([]string{"https://explicit.example.com"}))
}

func TestResolveFulcio_IssuersFromRefWhenUnset(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()},
		Spec: rhtasv1.FulcioSpec{
			Config: rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{
				{IssuerURL: "https://from-ref.example.com", ClientID: "sigstore"},
				{Issuer: "https://legacy-issuer.example.com", ClientID: "sigstore"},
			}},
		},
	}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.Url = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{
		TrustRootBinding: rhtasv1.TrustRootBinding{
			ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "fulcio", Namespace: t.Name()}},
		},
	}

	resolved, err := ResolveFulcio(t.Context(), c, t.Name(), binding)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.OIDCIssuers).To(Equal([]string{"https://from-ref.example.com", "https://legacy-issuer.example.com"}))
}

func TestResolveFulcio_IssuersFromAutodiscoveryWhenUnset(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()},
		Spec: rhtasv1.FulcioSpec{
			Config: rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{
				{IssuerURL: "https://from-autodiscovery.example.com", ClientID: "sigstore"},
			}},
		},
	}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.Url = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{}

	resolved, err := ResolveFulcio(t.Context(), c, t.Name(), binding)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Address).To(Equal("https://fulcio.internal.svc"))
	g.Expect(resolved.OIDCIssuers).To(Equal([]string{"https://from-autodiscovery.example.com"}))
}

func TestResolve_UnknownComponent(t *testing.T) {
	g := NewWithT(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := Resolve(t.Context(), c, t.Name(), ComponentKey("bogus"), rhtasv1.TrustRootBinding{})
	g.Expect(err).To(MatchError(ErrUnknownComponent))
}
