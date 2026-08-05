package action

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/config"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	haproxyRateLimitConnections   = "haproxy.router.openshift.io/rate-limit-connections"
	haproxyRateLimitConcurrentTCP = "haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp"
	haproxyRateLimitRateHTTP      = "haproxy.router.openshift.io/rate-limit-connections.rate-http"
	haproxyRateLimitRateTCP       = "haproxy.router.openshift.io/rate-limit-connections.rate-tcp"
)

type HAProxyThrottlingTestConfig[T apis.ConditionsAwareObject] struct {
	NewInstance    func() T
	NewService     func() *v1.Service
	NewAction      func() action.Action[T]
	SetAnnotations func(T, map[string]string)
	IngressName    string
	Namespace      string
	DefaultTCP     string
	DefaultHTTP    string
	DefaultRateTCP string
}

func RunHAProxyThrottlingTests[T apis.ConditionsAwareObject](t *testing.T, cfg HAProxyThrottlingTestConfig[T]) {
	t.Helper()
	ctx := context.TODO()

	t.Run("openshift with no annotations sets default HAProxy annotations", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, cfg.NewInstance())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "true"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, cfg.DefaultTCP))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateHTTP, cfg.DefaultHTTP))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateTCP, cfg.DefaultRateTCP))
	})

	t.Run("openshift with user annotations overriding HAProxy values", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{
			haproxyRateLimitConcurrentTCP: "10",
			haproxyRateLimitRateHTTP:      "20",
			haproxyRateLimitRateTCP:       "30",
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "true"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, "10"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateHTTP, "20"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateTCP, "30"))
	})

	t.Run("openshift with throttling disabled via annotations", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{
			haproxyRateLimitConnections: "false",
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "false"))
		g.Expect(ingress.Annotations).ToNot(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, cfg.DefaultTCP))
		g.Expect(ingress.Annotations).ToNot(HaveKeyWithValue(haproxyRateLimitRateHTTP, cfg.DefaultHTTP))
		g.Expect(ingress.Annotations).ToNot(HaveKeyWithValue(haproxyRateLimitRateTCP, cfg.DefaultRateTCP))
	})

	t.Run("non-openshift does not set throttling annotations regardless of user annotations", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = false

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{
			haproxyRateLimitConcurrentTCP: "10",
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitConnections))
		// user annotations are still applied even on non-OpenShift
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, "10"))
	})

	t.Run("opt-out combined with an explicit per-key override", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		cfg.SetAnnotations(instance, map[string]string{
			haproxyRateLimitConnections:   "false",
			haproxyRateLimitConcurrentTCP: "10",
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		// the master switch stays off...
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "false"))
		// ...but the user's own explicit annotation is still copied through, since
		// ensure.Annotations runs after EnsureIngressHAProxyThrottling in the chain
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, "10"))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateHTTP))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateTCP))
	})

	t.Run("opt-out and re-enable across successive reconciles", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())
		ingress := &networkingv1.Ingress{}

		a.Handle(ctx, instance)
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "true"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, cfg.DefaultTCP))

		cfg.SetAnnotations(instance, map[string]string{haproxyRateLimitConnections: "false"})
		a.Handle(ctx, instance)
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "false"))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitConcurrentTCP))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateHTTP))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateTCP))

		cfg.SetAnnotations(instance, nil)
		a.Handle(ctx, instance)
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConnections, "true"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitConcurrentTCP, cfg.DefaultTCP))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateHTTP, cfg.DefaultHTTP))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue(haproxyRateLimitRateTCP, cfg.DefaultRateTCP))
	})
}
