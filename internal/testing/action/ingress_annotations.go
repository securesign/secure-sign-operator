package action

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

type IngressUserMetadataTestConfig[T apis.ConditionsAwareObject] struct {
	NewInstance    func() T
	NewService     func() *v1.Service
	NewAction      func() action.Action[T]
	SetAnnotations func(T, map[string]string)
	SetLabels      func(T, map[string]string)
	IngressName    string
	Namespace      string
}

func RunIngressUserMetadataTests[T apis.ConditionsAwareObject](t *testing.T, cfg IngressUserMetadataTestConfig[T]) {
	t.Helper()
	ctx := context.TODO()
	ingressKey := types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}

	t.Run("user annotations are applied via SSA", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a", "custom/two": "b"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, ingressKey, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/two", "b"))
	})

	t.Run("annotation removed from the CR is deleted from the ingress on the next reconcile", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a", "custom/two": "b"})
		a.Handle(ctx, instance)

		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, ingressKey, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).ToNot(HaveKey("custom/two"))
	})

	t.Run("all annotations removed from the CR are deleted from the ingress", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a"})
		a.Handle(ctx, instance)

		cfg.SetAnnotations(instance, nil)
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, ingressKey, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).ToNot(HaveKey("custom/one"))
	})

	t.Run("user labels are applied via SSA", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetLabels(instance, map[string]string{"custom/label": "v1"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, ingressKey, ingress)).To(Succeed())
		g.Expect(ingress.Labels).To(HaveKeyWithValue("custom/label", "v1"))
	})

	t.Run("label removed from the CR is deleted from the ingress on the next reconcile", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetLabels(instance, map[string]string{"custom/one": "a", "custom/two": "b"})
		a.Handle(ctx, instance)

		cfg.SetLabels(instance, map[string]string{"custom/one": "a"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, ingressKey, ingress)).To(Succeed())
		g.Expect(ingress.Labels).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Labels).ToNot(HaveKey("custom/two"))
	})
}
