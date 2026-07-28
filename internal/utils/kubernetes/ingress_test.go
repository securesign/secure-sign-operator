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

	t.Run("nil throttling uses defaults, nil annotations", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{}

		g.Expect(EnsureIngressHAProxyThrottling(nil, defaults)(ingress)).To(gomega.Succeed())

		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})

	t.Run("nil throttling uses defaults, existing annotations preserved", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{
			ObjectMeta: v2.ObjectMeta{
				Annotations: map[string]string{"existing": "value"},
			},
		}

		g.Expect(EnsureIngressHAProxyThrottling(nil, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("existing", "value"))
		for k, v := range defaultAnnotations {
			g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(k, v))
		}
	})

	t.Run("partial override falls back to defaults for unset fields", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{}
		throttling := &rhtasv1.IngressThrottling{
			RateHTTP: utils.Pointer(int32(200)),
		}

		g.Expect(EnsureIngressHAProxyThrottling(throttling, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitConnections, annotations.HaproxyRateLimitConnectionsValue))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitConcurrentTCP, "100"))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitRateHTTP, "200"))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitRateTCP, "100"))
	})

	t.Run("full override uses all custom values", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ingress := &networkingv1.Ingress{}
		throttling := &rhtasv1.IngressThrottling{
			ConcurrentTCP: utils.Pointer(int32(10)),
			RateHTTP:      utils.Pointer(int32(20)),
			RateTCP:       utils.Pointer(int32(30)),
		}

		g.Expect(EnsureIngressHAProxyThrottling(throttling, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitConnections, annotations.HaproxyRateLimitConnectionsValue))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitConcurrentTCP, "10"))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitRateHTTP, "20"))
		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue(annotations.HaproxyRateLimitRateTCP, "30"))
	})

	t.Run("disabled removes existing throttling annotations", func(t *testing.T) {
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
		throttling := &rhtasv1.IngressThrottling{
			Enabled: utils.Pointer(false),
		}

		g.Expect(EnsureIngressHAProxyThrottling(throttling, defaults)(ingress)).To(gomega.Succeed())

		g.Expect(ingress.Annotations).To(gomega.HaveKeyWithValue("existing", "value"))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitConnections))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitConcurrentTCP))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitRateHTTP))
		g.Expect(ingress.Annotations).ToNot(gomega.HaveKey(annotations.HaproxyRateLimitRateTCP))
	})
}
