package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureIngressSpec(t *testing.T) {
	gomega.RegisterTestingT(t)
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
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			result, err := CreateOrUpdate(ctx, c,
				&networkingv1.Ingress{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureIngressSpec(ctx, c,
					v1.Service{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
					v1alpha1.ExternalAccess{
						Host: "host",
					},
					name),
			)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &networkingv1.Ingress{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Spec.Rules).To(gomega.HaveLen(1))
			gomega.Expect(existing.Spec.Rules[0].Host).To(gomega.Equal("host"))
			gomega.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths).To(gomega.HaveLen(1))
			gomega.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).To(gomega.Equal(name))
			gomega.Expect(existing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).To(gomega.Equal(name))

		})
	}
}
