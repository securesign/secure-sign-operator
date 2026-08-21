package kubernetes

import (
	"testing"

	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureIngressSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		objects []client.Object
		result  controllerutil.OperationResult
	}{
		{
			"create new object",
			[]client.Object{},
			controllerutil.OperationResultCreated,
		},
		{
			"update existing object",
			[]client.Object{
				&networkingv1.Ingress{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "test",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path:     "/",
												PathType: utils.Pointer(networkingv1.PathTypeImplementationSpecific),
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "fake",
														Port: networkingv1.ServiceBackendPort{
															Name: "fake",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&networkingv1.Ingress{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "host",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path:     "/",
												PathType: utils.Pointer(networkingv1.PathTypePrefix),
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: name,
														Port: networkingv1.ServiceBackendPort{
															Name: name,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			controllerutil.OperationResultNone,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			result, err := CreateOrUpdate(ctx, c,
				&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureIngressSpec(ctx, c,
					v1.Service{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
					rhtasv1.Ingress{
						Host: "host",
					},
					name),
			)
			g.Expect(err).ToNot(gomega.HaveOccurred())

			g.Expect(result).To(gomega.Equal(tt.result))

			existing := &networkingv1.Ingress{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())
			g.Expect(existing.Spec.Rules).To(gomega.HaveLen(1))
			g.Expect(existing.Spec.Rules[0].Host).To(gomega.Equal("host"))
			g.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths).To(gomega.HaveLen(1))
			g.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).To(gomega.Equal(name))
			g.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).To(gomega.Equal(name))
		})
	}
}

func TestEnsureIngressHAProxyThrottling(t *testing.T) {
	defaults := IngressThrottlingDefaults{
		ConcurrentTCP: 100,
		RateHTTP:      50,
		RateTCP:       100,
	}
	defaultAnnotations := map[string]string{
		annotations.HaproxyRateLimitConnections:   annotations.HaproxyRateLimitConnectionsValue,
		annotations.HaproxyRateLimitConcurrentTCP: "100",
		annotations.HaproxyRateLimitRateHTTP:      "50",
		annotations.HaproxyRateLimitRateTCP:       "100",
	}

	t.Run("nil user annotations applies defaults", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{}

		g.Expect(EnsureIngressHAProxyThrottling(nil, defaults)(ingress)).To(gomega.Succeed())

		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})

	t.Run("empty user annotations applies defaults, existing annotations preserved", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{
			ObjectMeta: v2.ObjectMeta{
				Annotations: map[string]string{"existing": "value"},
			},
		}

		g.Expect(EnsureIngressHAProxyThrottling(map[string]string{}, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("existing", "value"))
		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})

	t.Run("non-haproxy user annotations still applies defaults", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{}

		g.Expect(EnsureIngressHAProxyThrottling(map[string]string{"custom": "annotation"}, defaults)(ingress)).To(gomega.Succeed())

		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})

	t.Run("opt-out removes existing throttling annotations", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{
			ObjectMeta: v2.ObjectMeta{
				Annotations: map[string]string{
					"existing":                                "value",
					annotations.HaproxyRateLimitConnections:   annotations.HaproxyRateLimitConnectionsValue,
					annotations.HaproxyRateLimitConcurrentTCP: "100",
					annotations.HaproxyRateLimitRateHTTP:      "50",
					annotations.HaproxyRateLimitRateTCP:       "100",
				},
			},
		}

		g.Expect(EnsureIngressHAProxyThrottling(
			map[string]string{annotations.HaproxyRateLimitConnections: "false"},
			defaults,
		)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("existing", "value"))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitConnections))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitConcurrentTCP))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitRateHTTP))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitRateTCP))
	})

	t.Run("opt-out sentinel is an exact case-sensitive match, not a truthiness check", func(t *testing.T) {
		for _, value := range []string{"False", "FALSE", "0", " false", "false ", ""} {
			g := gomega.NewWithT(t)
			ingress := &networkingv1.Ingress{}

			g.Expect(EnsureIngressHAProxyThrottling(
				map[string]string{annotations.HaproxyRateLimitConnections: value},
				defaults,
			)(ingress)).To(gomega.Succeed())

			for k, v := range defaultAnnotations {
				g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v), "value %q should not disable throttling", value)
			}
		}
	})

	t.Run("re-enabling after opt-out restores defaults", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{
			ObjectMeta: v2.ObjectMeta{
				Annotations: map[string]string{
					"existing": "value",
				},
			},
		}

		// first reconcile: user opts out
		g.Expect(EnsureIngressHAProxyThrottling(
			map[string]string{annotations.HaproxyRateLimitConnections: "false"},
			defaults,
		)(ingress)).To(gomega.Succeed())
		for _, key := range haproxyThrottlingAnnotationKeys {
			g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(key))
		}

		// second reconcile: user removes the opt-out annotation entirely
		g.Expect(EnsureIngressHAProxyThrottling(map[string]string{}, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("existing", "value"))
		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})
}

func TestApplyIngressUserMetadata(t *testing.T) {
	t.Run("user annotations and labels are applied to the ingress", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ctx := t.Context()
		c := testAction.FakeClientBuilder().
			WithObjects(&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}}).
			Build()

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default",
			map[string]string{"custom/ann": "a"},
			map[string]string{"custom/label": "b"},
		)).To(gomega.Succeed())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, ingress)).To(gomega.Succeed())
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("custom/ann", "a"))
		g.Expect(ingress.Labels).To(gomega.HaveKeyWithValue("custom/label", "b"))
	})

	t.Run("removed annotation key is deleted on next apply", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ctx := t.Context()
		c := testAction.FakeClientBuilder().
			WithObjects(&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}}).
			Build()

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default",
			map[string]string{"custom/one": "a", "custom/two": "b"}, nil,
		)).To(gomega.Succeed())

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default",
			map[string]string{"custom/one": "a"}, nil,
		)).To(gomega.Succeed())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, ingress)).To(gomega.Succeed())
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey("custom/two"))
	})

	t.Run("all annotations removed cleans up all SSA-owned keys", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ctx := t.Context()
		c := testAction.FakeClientBuilder().
			WithObjects(&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}}).
			Build()

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default",
			map[string]string{"custom/one": "a"}, nil,
		)).To(gomega.Succeed())

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default", nil, nil)).To(gomega.Succeed())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, ingress)).To(gomega.Succeed())
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey("custom/one"))
	})

	t.Run("nil maps are safe and equivalent to empty", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ctx := t.Context()
		c := testAction.FakeClientBuilder().
			WithObjects(&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}}).
			Build()

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default", nil, nil)).To(gomega.Succeed())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, ingress)).To(gomega.Succeed())
	})

	t.Run("removed label key is deleted on next apply", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ctx := t.Context()
		c := testAction.FakeClientBuilder().
			WithObjects(&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}}).
			Build()

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default", nil,
			map[string]string{"custom/one": "a", "custom/two": "b"},
		)).To(gomega.Succeed())

		g.Expect(ApplyIngressUserMetadata(ctx, c, name, "default", nil,
			map[string]string{"custom/one": "a"},
		)).To(gomega.Succeed())

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, ingress)).To(gomega.Succeed())
		g.Expect(ingress.Labels).To(gomega.HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Labels).ToNot(gomega.HaveKey("custom/two"))
	})
}
