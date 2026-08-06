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

type IngressAnnotationTestConfig[T apis.ConditionsAwareObject] struct {
	NewInstance    func() T
	NewService     func() *v1.Service
	NewAction      func() action.Action[T]
	SetAnnotations func(T, map[string]string)
	IngressName    string
	Namespace      string
}

// RunIngressAnnotationRemovalTests verifies that an annotation key removed from
// the CR between reconciles is deleted from the live Ingress, not left stuck.
func RunIngressAnnotationRemovalTests[T apis.ConditionsAwareObject](t *testing.T, cfg IngressAnnotationTestConfig[T]) {
	t.Helper()
	ctx := context.TODO()

	t.Run("one annotation removed from the CR is deleted from the ingress on the next reconcile", func(t *testing.T) {
		g := NewWithT(t)
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a", "custom/two": "b"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/two", "b"))

		cfg.SetAnnotations(instance, map[string]string{"custom/one": "a"})
		a.Handle(ctx, instance)

		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
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
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).ToNot(HaveKey("custom/one"))
	})
}
