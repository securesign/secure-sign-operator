package kubernetes

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/annotations"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

// POC: no CreateOrUpdate, no per-component pruning closure -- dropping a
// label from fn's input is enough for SSA to remove it on re-apply.
func TestApplySSA_PoC_AutoPrunesDroppedLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()
	c := newFakeClient(t)

	ingress := func() *networkingv1.Ingress {
		return &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"}}
	}

	changed, err := ApplySSA(ctx, c, ingress(), func(i *networkingv1.Ingress) error {
		i.Labels = map[string]string{"app": "demo", "extra": "keep-me-out-next-time"}
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(changed).To(BeTrue(), "create must report changed=true")

	got := &networkingv1.Ingress{}
	g.Expect(c.Get(ctx, types.NamespacedName{Name: "demo", Namespace: "ns"}, got)).To(Succeed())
	g.Expect(got.Labels).To(HaveKeyWithValue("app", "demo"))
	g.Expect(got.Labels).To(HaveKeyWithValue("extra", "keep-me-out-next-time"))

	// NOTE: a "second reconcile, identical desired state -> changed=false"
	// case belongs here too, but the fake client doesn't emulate SSA's
	// no-op detection -- it bumps resourceVersion on every Patch regardless
	// of content, unlike a real API server. That must be verified against
	// envtest/a real cluster, not this fake-client test.

	// fn no longer sets "extra" -- no manual pruning
	// closure needed, SSA releases it because this owner no longer claims it.
	changed, err = ApplySSA(ctx, c, ingress(), func(i *networkingv1.Ingress) error {
		i.Labels = map[string]string{"app": "demo"}
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(changed).To(BeTrue(), "dropping a previously-owned key must report changed=true")

	g.Expect(c.Get(ctx, types.NamespacedName{Name: "demo", Namespace: "ns"}, got)).To(Succeed())
	g.Expect(got.Labels).To(HaveKeyWithValue("app", "demo"))
	g.Expect(got.Labels).NotTo(HaveKey("extra"))
}

// POC: fn never runs, and no Patch is sent, once the object is paused.
func TestApplySSA_PoC_RespectsPauseAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()
	c := newFakeClient(t)

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo", Namespace: "ns",
			Annotations: map[string]string{annotations.PausedReconciliation: "true"},
			Labels:      map[string]string{"untouched": "true"},
		},
	}
	g.Expect(c.Create(ctx, existing)).To(Succeed())

	called := false
	changed, err := ApplySSA(ctx, c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"}},
		func(cm *corev1.ConfigMap) error {
			called = true
			cm.Labels = map[string]string{"should": "not-apply"}
			return nil
		})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(called).To(BeFalse())
	g.Expect(changed).To(BeFalse())

	got := &corev1.ConfigMap{}
	g.Expect(c.Get(ctx, types.NamespacedName{Name: "demo", Namespace: "ns"}, got)).To(Succeed())
	g.Expect(got.Labels).To(HaveKeyWithValue("untouched", "true"))
}

// POC: a field owned by another field manager, that we never mention in fn,
// must survive our Apply untouched -- this is the co-ownership property that
// lets ApplySSA use a bare object instead of a Get'd live copy.
func TestApplySSA_PoC_OtherManagersFieldsSurvive(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()
	c := newFakeClient(t)

	other := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"},
	}
	other.Annotations = map[string]string{"other-controller/owns-me": "true"}
	g.Expect(c.Patch(ctx, other, client.Apply, client.FieldOwner("other-controller"), client.ForceOwnership)).To(Succeed())

	changed, err := ApplySSA(ctx, c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"}},
		func(cm *corev1.ConfigMap) error {
			cm.Labels = map[string]string{"app": "demo"}
			return nil
		})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(changed).To(BeTrue())

	got := &corev1.ConfigMap{}
	g.Expect(c.Get(ctx, types.NamespacedName{Name: "demo", Namespace: "ns"}, got)).To(Succeed())
	g.Expect(got.Labels).To(HaveKeyWithValue("app", "demo"))
	// untouched by us, owned by "other-controller" -- must survive
	g.Expect(got.Annotations).To(HaveKeyWithValue("other-controller/owns-me", "true"))
}

// POC: a field with no SSA owner at all -- e.g. a user's manual `kubectl
// label`, or any object created via plain Create/Update before migrating to
// ApplySSA -- must also survive untouched if fn doesn't mention it.
func TestApplySSA_PoC_ManuallySetFieldSurvives(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()
	c := newFakeClient(t)

	manual := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo", Namespace: "ns",
			Labels: map[string]string{"manual": "true"},
		},
	}
	g.Expect(c.Create(ctx, manual)).To(Succeed())

	changed, err := ApplySSA(ctx, c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "ns"}},
		func(cm *corev1.ConfigMap) error {
			cm.Labels = map[string]string{"app": "demo"}
			return nil
		})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(changed).To(BeTrue())

	got := &corev1.ConfigMap{}
	g.Expect(c.Get(ctx, types.NamespacedName{Name: "demo", Namespace: "ns"}, got)).To(Succeed())
	g.Expect(got.Labels).To(HaveKeyWithValue("app", "demo"))
	// never claimed by any field manager -- must survive
	g.Expect(got.Labels).To(HaveKeyWithValue("manual", "true"))
}
