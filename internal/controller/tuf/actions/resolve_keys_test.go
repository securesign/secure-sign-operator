package actions

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed testdata/public_key.pem
var testPEM string

// tufInstance builds a "tuf"/"default" Tuf instance with explicit trust root
// bindings. Ctlog always autodiscovers (no test needs an explicit Ctlog
// binding); a nil tsa excludes TSA from the trust root entirely.
func tufInstance(rekor []rhtasv1.TrustRootBinding, fulcio []rhtasv1.TrustRootBindingWithOIDC, tsa *[]rhtasv1.TrustRootBinding) *rhtasv1.Tuf {
	return &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{Name: "tuf", Namespace: "default"},
		Spec: rhtasv1.TufSpec{
			Rekor:  rekor,
			Fulcio: fulcio,
			Tsa:    tsa,
		},
		Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
			{Type: constants.ReadyCondition, Reason: state.Pending.String(), Status: metav1.ConditionFalse},
		}},
	}
}

// noTSA builds a "tuf" instance with TSA excluded, and rekor/ctlog/fulcio all
// zero-value bindings (autodiscover) unless overridden by the caller.
func noTSA() *rhtasv1.Tuf {
	return tufInstance(nil, nil, nil)
}

func readyRekor(ns string) *rhtasv1.Rekor {
	r := &rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns}}
	r.Status.PublicKey = testPEM
	r.Status.Url = "https://rekor.internal.svc"
	r.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return r
}

func readyCTlog() *rhtasv1.CTlog {
	c := &rhtasv1.CTlog{ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"}}
	c.Status.Logs = []rhtasv1.CTlogLogStatus{
		{
			Prefix:    "test-log",
			Active:    true,
			PublicKey: testPEM,
		},
	}
	c.Status.Url = "https://ctlog.internal.svc"
	c.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return c
}

func readyFulcio(ns string) *rhtasv1.Fulcio {
	f := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: ns},
		Spec: rhtasv1.FulcioSpec{
			Config: rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{{ClientID: "t", Issuer: "t"}}},
			Signer: rhtasv1.FulcioSigner{Type: "file", CertificateChain: rhtasv1.FulcioCertificateChain{CommonName: "t", OrganizationName: "t", OrganizationEmail: "t@t"}},
		},
	}
	f.Status.CertificateChain = testPEM
	f.Status.Url = "https://fulcio.internal.svc"
	f.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return f
}

func readyTSA(ns string) *rhtasv1.TimestampAuthority {
	t := &rhtasv1.TimestampAuthority{ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: ns}}
	t.Status.CertificateChain = testPEM
	t.Status.Url = "https://tsa.internal.svc"
	t.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return t
}

func userRef(name, key string) *rhtasv1.SecretKeySelector {
	return &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: name},
		Key:                  key,
	}
}

// explicitSecret builds the Secret a SecretRef-carrying binding points at, so
// material resolution for that binding succeeds without needing autodiscovery.
func explicitSecret(ns, name, key string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string][]byte{key: []byte(testPEM)},
	}
}

// explicitBinding returns a binding with an explicit SecretRef for material and an
// explicit URL for address, so tests exercising material behavior don't need a
// live component object in the fake client just to satisfy address resolution.
func explicitBinding(secretName, key string) rhtasv1.TrustRootBinding {
	return rhtasv1.TrustRootBinding{
		ServiceReference: rhtasv1.ServiceReference{URL: "http://" + secretName + ".fakeserver.com"},
		SecretRef:        userRef(secretName, key),
	}
}

func tufSecretName() string {
	return fmt.Sprintf(tufKeysSecretFormat, "tuf")
}

func TestResolveKeys_Handle(t *testing.T) {
	const ns = "default"

	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1.Tuf, client.Client)
	}

	tests := []struct {
		name     string
		instance func() *rhtasv1.Tuf
		objects  []client.Object
		want     want
	}{
		{
			name:     "all autodiscovered, no TSA",
			instance: func() *rhtasv1.Tuf { return noTSA() },
			objects:  []client.Object{readyRekor(ns), readyCTlog(), readyFulcio(ns)},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(3))
					for _, k := range instance.Status.Keys {
						g.Expect(k.SecretRef.Name).To(Equal(tufSecretName()))
					}
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, trustroot.Rekor.String())).To(BeTrue())
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, trustroot.CTFE.String())).To(BeTrue())
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, trustroot.Fulcio.String())).To(BeTrue())

					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveLen(3))
				},
			},
		},
		{
			name: "all four components including TSA",
			instance: func() *rhtasv1.Tuf {
				return tufInstance(nil, nil, &[]rhtasv1.TrustRootBinding{})
			},
			objects: []client.Object{readyRekor(ns), readyCTlog(), readyFulcio(ns), readyTSA(ns)},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(4))
					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveLen(4))
				},
			},
		},
		{
			name: "user-provided secretRef passes through untouched",
			instance: func() *rhtasv1.Tuf {
				return tufInstance(
					[]rhtasv1.TrustRootBinding{explicitBinding("my-secret", "pub")}, nil, nil)
			},
			objects: []client.Object{readyCTlog(), readyFulcio(ns), explicitSecret(ns, "my-secret", "pub")},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys[0].SecretRef).To(Equal(userRef("my-secret", "pub")))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, trustroot.Rekor.String())).To(BeTrue())
				},
			},
		},
		{
			name: "mixed — provided and autodiscovery",
			instance: func() *rhtasv1.Tuf {
				return tufInstance(
					[]rhtasv1.TrustRootBinding{explicitBinding("user-rekor", "key")},
					[]rhtasv1.TrustRootBindingWithOIDC{{
						TrustRootBinding: explicitBinding("user-fulcio", "cert"),
						OIDCIssuers:      []string{"https://oidc.fakeserver.com"},
					}},
					nil)
			},
			objects: []client.Object{readyCTlog(), explicitSecret(ns, "user-rekor", "key"), explicitSecret(ns, "user-fulcio", "cert")},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(3))
					byName := map[string]rhtasv1.TufKeyStatus{}
					for _, k := range instance.Status.Keys {
						byName[k.Name] = k
					}
					g.Expect(byName[trustroot.Rekor.String()].SecretRef.Name).To(Equal("user-rekor"))
					g.Expect(byName[trustroot.CTFE.String()].SecretRef.Name).To(Equal(tufSecretName()))
					g.Expect(byName[trustroot.Fulcio.String()].SecretRef.Name).To(Equal("user-fulcio"))

					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveLen(1))
					g.Expect(secret.Data).To(HaveKey(trustroot.CTFE.String()))
				},
			},
		},
		{
			name: "TSA excluded, stale status and condition are dropped",
			instance: func() *rhtasv1.Tuf {
				instance := noTSA()
				instance.Status.Keys = []rhtasv1.TufKeyStatus{
					{Name: trustroot.TSA.String(), SecretRef: userRef("stale-tsa", "key")},
				}
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type: trustroot.TSA.String(), Status: metav1.ConditionTrue, Reason: state.Ready.String(),
				})
				return instance
			},
			objects: []client.Object{readyRekor(ns), readyCTlog(), readyFulcio(ns)},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					for _, k := range instance.Status.Keys {
						g.Expect(k.Name).ToNot(Equal(trustroot.TSA.String()))
					}
					g.Expect(meta.FindStatusCondition(instance.Status.Conditions, trustroot.TSA.String())).To(BeNil())
				},
			},
		},
		{
			name: "no changes needed, continues without persisting",
			instance: func() *rhtasv1.Tuf {
				instance := noTSA()
				instance.Status.Keys = []rhtasv1.TufKeyStatus{
					{Name: trustroot.Rekor.String(), SecretRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: tufSecretName()}, Key: trustroot.Rekor.String()}},
					{Name: trustroot.CTFE.String(), SecretRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: tufSecretName()}, Key: trustroot.CTFE.String()}},
					{Name: trustroot.Fulcio.String(), SecretRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: tufSecretName()}, Key: trustroot.Fulcio.String()}},
				}
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: trustroot.Rekor.String(), Status: metav1.ConditionTrue, Reason: state.Ready.String()})
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: trustroot.CTFE.String(), Status: metav1.ConditionTrue, Reason: state.Ready.String()})
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: trustroot.Fulcio.String(), Status: metav1.ConditionTrue, Reason: state.Ready.String()})
				return instance
			},
			objects: []client.Object{readyRekor(ns), readyCTlog(), readyFulcio(ns)},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(3))
				},
			},
		},
		{
			name:     "component not ready — requeue",
			instance: func() *rhtasv1.Tuf { return noTSA() },
			objects: []client.Object{
				&rhtasv1.Rekor{
					ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns},
					Status: rhtasv1.RekorStatus{Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
					}},
				},
				readyCTlog(), readyFulcio(ns),
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, trustroot.Rekor.String())).To(BeTrue())
				},
			},
		},
		{
			name:     "no component instance — requeue",
			instance: func() *rhtasv1.Tuf { return noTSA() },
			objects:  []client.Object{readyCTlog(), readyFulcio(ns)},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, trustroot.Rekor.String())).To(BeTrue())
				},
			},
		},
		{
			name:     "component ready but trust material empty — requeue",
			instance: func() *rhtasv1.Tuf { return noTSA() },
			objects: []client.Object{
				&rhtasv1.Rekor{
					ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns},
					Status: rhtasv1.RekorStatus{
						PublicKey: "",
						Url:       "https://rekor.internal.svc",
						Conditions: []metav1.Condition{
							{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
						},
					},
				},
				readyCTlog(), readyFulcio(ns),
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					cond := meta.FindStatusCondition(instance.Status.Conditions, trustroot.Rekor.String())
					g.Expect(cond.Message).ToNot(BeEmpty())
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := tt.instance()
			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			for _, obj := range tt.objects {
				builder = builder.WithObjects(obj)
			}
			c := builder.Build()
			a := testAction.PrepareAction(c, NewResolveKeysAction())

			result := a.Handle(t.Context(), instance)

			g.Expect(result).To(Equal(tt.want.result))
			if tt.want.verify != nil {
				tt.want.verify(g, instance, c)
			}
		})
	}
}

func TestResolveKeys_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  *rhtasv1.Tuf
		canHandle bool
	}{
		{
			name:      "pending state",
			instance:  noTSA(),
			canHandle: true,
		},
		{
			name: "below pending state",
			instance: &rhtasv1.Tuf{
				Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Reason: state.NotDefined.String(), Status: metav1.ConditionFalse},
				}},
			},
			canHandle: false,
		},
		{
			name:      "no ReadyCondition",
			instance:  &rhtasv1.Tuf{},
			canHandle: false,
		},
		{
			name: "ready state, still re-resolved every eligible reconcile",
			instance: &rhtasv1.Tuf{
				Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Reason: state.Ready.String(), Status: metav1.ConditionTrue},
				}},
			},
			canHandle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolveKeysAction())
			g := NewWithT(t)
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}
