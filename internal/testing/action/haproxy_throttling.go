package action

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/config"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
	SetThrottling  func(T, *rhtasv1.IngressThrottling)
	IngressName    string
	Namespace      string
	DefaultTCP     string
	DefaultHTTP    string
	DefaultRateTCP string
}

func RunHAProxyThrottlingTests[T apis.ConditionsAwareObject](t *testing.T, cfg HAProxyThrottlingTestConfig[T]) {
	t.Helper()
	ctx := context.TODO()

	t.Run("openshift with nil throttling sets default annotations", func(t *testing.T) {
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

	t.Run("openshift with custom throttling values in CR", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		cfg.SetThrottling(instance, &rhtasv1.IngressThrottling{
			ConcurrentTCP: ptr.To(int32(10)),
			RateHTTP:      ptr.To(int32(20)),
			RateTCP:       ptr.To(int32(30)),
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

	t.Run("openshift with throttling disabled in CR", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		instance := cfg.NewInstance()
		cfg.SetThrottling(instance, &rhtasv1.IngressThrottling{
			Enabled: ptr.To(false),
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitConnections))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitConcurrentTCP))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateHTTP))
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitRateTCP))
	})

	t.Run("non-openshift does not set throttling annotations regardless of CR config", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = false

		instance := cfg.NewInstance()
		cfg.SetThrottling(instance, &rhtasv1.IngressThrottling{
			ConcurrentTCP: ptr.To(int32(10)),
		})

		c := FakeClientBuilder().WithObjects(cfg.NewService()).Build()
		a := PrepareAction(c, cfg.NewAction())

		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: cfg.IngressName, Namespace: cfg.Namespace}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).ToNot(HaveKey(haproxyRateLimitConnections))
	})
}
