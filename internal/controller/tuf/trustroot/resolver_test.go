package trustroot

import (
	"testing"
	"time"

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

	start := metav1.NewMicroTime(time.Unix(1_700_000_000, 0).UTC())
	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{URL: "http://rekor.fakeserver.com"},
		SecretRef:        &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"}, Key: "rekor.pub"},
		ValidFor:         &rhtasv1.ValidFor{Start: &start},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Rekor: []rhtasv1.TrustRootBinding{binding}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Rekor).To(HaveLen(1))
	g.Expect(resolved.Rekor[0].BaseURL).To(Equal("http://rekor.fakeserver.com"))
	g.Expect(resolved.Rekor[0].TrustMaterial).To(Equal([]byte(testPEM)))
	g.Expect(resolved.Rekor[0].ValidFor).To(Equal(*binding.ValidFor))
}

func TestResolve_RefBased(t *testing.T) {
	g := NewWithT(t)
	rekor := &rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: t.Name()}}
	rekor.Status.PublicKey = testPEM
	rekor.Status.URL = "https://rekor.internal.svc"
	rekor.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rekor).WithStatusSubresource(rekor).Build()
	g.Expect(c.Status().Update(t.Context(), rekor)).To(Succeed())

	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "rekor", Namespace: t.Name()}},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Rekor: []rhtasv1.TrustRootBinding{binding}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Rekor).To(HaveLen(1))
	g.Expect(resolved.Rekor[0].BaseURL).To(Equal("https://rekor.internal.svc"))
	g.Expect(resolved.Rekor[0].TrustMaterial).To(Equal([]byte(testPEM)))
}

func TestResolve_UsesActiveCTlog(t *testing.T) {
	g := NewWithT(t)
	ctlog := &rhtasv1.CTlog{ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: t.Name()}}
	ctlog.Status.Logs = []rhtasv1.CTlogLogStatus{
		{
			Prefix:    "test-log",
			Active:    true,
			PublicKey: testPEM,
		},
	}
	ctlog.Status.URL = "https://ctlog.internal.svc"
	ctlog.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ctlog).WithStatusSubresource(ctlog).Build()
	g.Expect(c.Status().Update(t.Context(), ctlog)).To(Succeed())

	binding := rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "ctlog", Namespace: t.Name()}},
	}
	resolved, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Ctlog: []rhtasv1.TrustRootBinding{binding}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Ctlog).To(HaveLen(1))
	g.Expect(resolved.Ctlog[0].BaseURL).To(Equal("https://ctlog.internal.svc"))
	g.Expect(resolved.Ctlog[0].TrustMaterial).To(Equal([]byte(testPEM)))
}

func TestResolve_RejectsUnreadyReference(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()}}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.URL = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{
		TrustRootBinding: rhtasv1.TrustRootBinding{
			ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "fulcio", Namespace: t.Name()}},
		},
	}

	_, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Fulcio: []rhtasv1.TrustRootBindingWithOIDC{binding}})
	g.Expect(err).To(MatchError(ErrResolveAddress))
}

func TestResolve_ExplicitOIDCIssuersWin(t *testing.T) {
	g := NewWithT(t)
	fulcio := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: t.Name()},
		Spec: rhtasv1.FulcioSpec{
			Config: rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{{IssuerURL: "https://from-ref.example.com", ClientID: "sigstore"}}},
		},
	}
	fulcio.Status.CertificateChain = testPEM
	fulcio.Status.URL = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{
		TrustRootBinding: rhtasv1.TrustRootBinding{
			ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "fulcio", Namespace: t.Name()}},
		},
		OIDCIssuers: []string{"https://explicit.example.com"},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Fulcio: []rhtasv1.TrustRootBindingWithOIDC{binding}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Fulcio).To(HaveLen(1))
	g.Expect(resolved.Fulcio[0].OIDCIssuers).To(Equal([]string{"https://explicit.example.com"}))
}

func TestResolve_IssuersFromRefWhenUnset(t *testing.T) {
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
	fulcio.Status.URL = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	binding := rhtasv1.TrustRootBindingWithOIDC{
		TrustRootBinding: rhtasv1.TrustRootBinding{
			ServiceReference: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Name: "fulcio", Namespace: t.Name()}},
		},
	}

	resolved, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Fulcio: []rhtasv1.TrustRootBindingWithOIDC{binding}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved.Fulcio).To(HaveLen(1))
	g.Expect(resolved.Fulcio[0].OIDCIssuers).To(Equal([]string{"https://from-ref.example.com", "https://legacy-issuer.example.com"}))
}

func TestResolve_RejectsAutodiscovery(t *testing.T) {
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
	fulcio.Status.URL = "https://fulcio.internal.svc"
	fulcio.Status.Conditions = []metav1.Condition{readyCondition()}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fulcio).WithStatusSubresource(fulcio).Build()
	g.Expect(c.Status().Update(t.Context(), fulcio)).To(Succeed())

	_, err := Resolve(t.Context(), c, t.Name(), rhtasv1.Components{Fulcio: []rhtasv1.TrustRootBindingWithOIDC{{}}})
	g.Expect(err).To(MatchError(ErrResolveAddress))
}
