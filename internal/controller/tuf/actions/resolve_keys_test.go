package actions

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const testPEM = "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEtest\n-----END PUBLIC KEY-----\n"

func tufInstance(name, ns string, keys []rhtasv1.TufKey) *rhtasv1.Tuf {
	return &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       rhtasv1.TufSpec{Keys: keys},
		Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
			{Type: constants.ReadyCondition, Reason: state.Pending.String(), Status: metav1.ConditionFalse},
		}},
	}
}

func readyRekor(ns string) *rhtasv1.Rekor {
	r := &rhtasv1.Rekor{ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns}}
	r.Status.PublicKey = testPEM
	r.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return r
}

func readyCTlog() *rhtasv1.CTlog {
	c := &rhtasv1.CTlog{ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"}}
	c.Status.PublicKey = testPEM
	c.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return c
}

func readyFulcio(ns string) *rhtasv1.Fulcio {
	f := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: ns},
		Spec: rhtasv1.FulcioSpec{
			Config:      rhtasv1.FulcioConfig{OIDCIssuers: []rhtasv1.OIDCIssuer{{ClientID: "t", Issuer: "t"}}},
			Certificate: rhtasv1.FulcioCert{CommonName: "t", OrganizationName: "t", OrganizationEmail: "t@t"},
		},
	}
	f.Status.CertificateChain = testPEM
	f.Status.Conditions = []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}
	return f
}

func readyTSA(ns string) *rhtasv1.TimestampAuthority {
	t := &rhtasv1.TimestampAuthority{ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: ns}}
	t.Status.CertificateChain = testPEM
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

func tufSecretName() string {
	return fmt.Sprintf(tufKeysSecretFormat, "tuf")
}

func TestResolveKeys_Handle(t *testing.T) {
	const ns = "default"
	const instanceName = "tuf"

	type env struct {
		keys    []rhtasv1.TufKey
		status  *rhtasv1.TufStatus
		objects []client.Object
	}
	type want struct {
		result     *action.Result
		isTerminal bool
		verify     func(Gomega, *rhtasv1.Tuf, client.Client)
	}

	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "single autodiscovery key",
			env: env{
				keys:    []rhtasv1.TufKey{{Name: "rekor.pub"}},
				objects: []client.Object{readyRekor(ns)},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(1))
					g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal(tufSecretName()))
					g.Expect(instance.Status.Keys[0].SecretRef.Key).To(Equal("rekor.pub"))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())

					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveKeyWithValue("rekor.pub", []byte(testPEM)))
				},
			},
		},
		{
			name: "multiple autodiscovery keys",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub"},
					{Name: "ctfe.pub"},
				},
				objects: []client.Object{readyRekor(ns), readyCTlog()},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(2))
					for _, k := range instance.Status.Keys {
						g.Expect(k.SecretRef.Name).To(Equal(tufSecretName()))
					}
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "ctfe.pub")).To(BeTrue())

					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveKey("rekor.pub"))
					g.Expect(secret.Data).To(HaveKey("ctfe.pub"))
				},
			},
		},
		{
			name: "all four autodiscovery keys",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub"},
					{Name: "ctfe.pub"},
					{Name: "fulcio_v1.crt.pem"},
					{Name: "tsa.certchain.pem"},
				},
				objects: []client.Object{readyRekor(ns), readyCTlog(), readyFulcio(ns), readyTSA(ns)},
			},
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
			name: "user-provided key passes through",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub", SecretRef: userRef("my-secret", "pub")},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(1))
					g.Expect(instance.Status.Keys[0].SecretRef).To(Equal(userRef("my-secret", "pub")))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())

					secret := &v1.Secret{}
					err := c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)
					g.Expect(err).To(HaveOccurred())
				},
			},
		},
		{
			name: "mixed keys — provided and autodiscovery",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub", SecretRef: userRef("user-rekor", "key")},
					{Name: "ctfe.pub"},
					{Name: "fulcio_v1.crt.pem", SecretRef: userRef("user-fulcio", "cert")},
				},
				objects: []client.Object{readyCTlog()},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, c client.Client) {
					g.Expect(instance.Status.Keys).To(HaveLen(3))

					g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("user-rekor"))
					g.Expect(instance.Status.Keys[1].SecretRef.Name).To(Equal(tufSecretName()))
					g.Expect(instance.Status.Keys[1].SecretRef.Key).To(Equal("ctfe.pub"))
					g.Expect(instance.Status.Keys[2].SecretRef.Name).To(Equal("user-fulcio"))

					secret := &v1.Secret{}
					g.Expect(c.Get(t.Context(), client.ObjectKey{Namespace: ns, Name: tufSecretName()}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveLen(1))
					g.Expect(secret.Data).To(HaveKey("ctfe.pub"))
				},
			},
		},
		{
			name: "user updates SecretRef",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub", SecretRef: userRef("new", "key")},
				},
				status: &rhtasv1.TufStatus{
					Keys: []rhtasv1.TufKeyStatus{
						{Name: "rekor.pub", SecretRef: userRef("old", "key")},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))
				},
			},
		},
		{
			name: "autodiscovery replaces stale status",
			env: env{
				keys: []rhtasv1.TufKey{{Name: "ctfe.pub"}},
				status: &rhtasv1.TufStatus{
					Keys: []rhtasv1.TufKeyStatus{
						{Name: "ctfe.pub", SecretRef: userRef("old-secret", "key")},
					},
				},
				objects: []client.Object{readyCTlog()},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal(tufSecretName()))
					g.Expect(instance.Status.Keys[0].SecretRef.Key).To(Equal("ctfe.pub"))
				},
			},
		},
		{
			name: "component not ready — requeue",
			env: env{
				keys: []rhtasv1.TufKey{{Name: "rekor.pub"}},
				objects: []client.Object{
					&rhtasv1.Rekor{
						ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns},
						Status: rhtasv1.RekorStatus{Conditions: []metav1.Condition{
							{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
						}},
					},
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
				},
			},
		},
		{
			name: "no component instance — requeue",
			env: env{
				keys: []rhtasv1.TufKey{{Name: "rekor.pub"}},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
					cond := meta.FindStatusCondition(instance.Status.Conditions, "rekor.pub")
					g.Expect(cond.Message).To(ContainSubstring(ErrNoReadyComponent.Error()))
				},
			},
		},
		{
			name: "component ready but trust material empty — requeue",
			env: env{
				keys: []rhtasv1.TufKey{{Name: "rekor.pub"}},
				objects: []client.Object{
					&rhtasv1.Rekor{
						ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: ns},
						Status: rhtasv1.RekorStatus{
							PublicKey: "",
							Conditions: []metav1.Condition{
								{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
							},
						},
					},
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					cond := meta.FindStatusCondition(instance.Status.Conditions, "rekor.pub")
					g.Expect(cond.Message).To(ContainSubstring(ErrTrustMaterialNotReady.Error()))
				},
			},
		},
		{
			name: "first key fails — stops before processing second",
			env: env{
				keys: []rhtasv1.TufKey{
					{Name: "rekor.pub"},
					{Name: "ctfe.pub"},
				},
				objects: []client.Object{readyCTlog()},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, instance *rhtasv1.Tuf, _ client.Client) {
					g.Expect(instance.Status.Keys).To(BeEmpty())
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := tufInstance(instanceName, ns, tt.env.keys)
			if tt.env.status != nil {
				instance.Status.Keys = tt.env.status.Keys
			}

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			for _, obj := range tt.env.objects {
				builder = builder.WithObjects(obj)
			}
			c := builder.Build()
			a := testAction.PrepareAction(c, NewResolveKeysAction())

			result := a.Handle(t.Context(), instance)

			if tt.want.isTerminal {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).To(HaveOccurred())
				g.Expect(errors.Is(result.Err, reconcile.TerminalError(nil))).To(BeTrue())
				return
			}
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
			name:      "pending state, keys unresolved",
			instance:  tufInstance("tuf", "default", []rhtasv1.TufKey{{Name: "rekor.pub"}}),
			canHandle: true,
		},
		{
			name: "below pending state",
			instance: &rhtasv1.Tuf{
				Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{{Name: "rekor.pub"}}},
				Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Reason: state.NotDefined.String(), Status: metav1.ConditionFalse},
				}},
			},
			canHandle: false,
		},
		{
			name: "no ReadyCondition",
			instance: &rhtasv1.Tuf{
				Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{{Name: "rekor.pub"}}},
			},
			canHandle: false,
		},
		{
			name: "keys already resolved — provided",
			instance: &rhtasv1.Tuf{
				Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
					{Name: "rekor.pub", SecretRef: userRef("s", "k")},
				}},
				Status: rhtasv1.TufStatus{
					Keys: []rhtasv1.TufKeyStatus{
						{Name: "rekor.pub", SecretRef: userRef("s", "k")},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Pending.String(), Status: metav1.ConditionFalse},
					},
				},
			},
			canHandle: false,
		},
		{
			name: "autodiscovery keys resolved — spec nil matches any status",
			instance: &rhtasv1.Tuf{
				Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
					{Name: "rekor.pub"},
				}},
				Status: rhtasv1.TufStatus{
					Keys: []rhtasv1.TufKeyStatus{
						{Name: "rekor.pub", SecretRef: userRef("tuf-keys-tuf", "rekor.pub")},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Pending.String(), Status: metav1.ConditionFalse},
					},
				},
			},
			canHandle: false,
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
