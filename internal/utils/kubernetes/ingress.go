package kubernetes

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	v12 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnsureIngressSpec(ctx context.Context, cli client.Client, svc v12.Service, conf rhtasv1.Ingress, port string) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {
		path := networkingv1.PathTypePrefix
		host := conf.Host

		if host == "" {
			var err error
			if host, err = CalculateHostname(ctx, cli, svc.Name, svc.Namespace); err != nil {
				return err
			}
		}

		spec := &ingress.Spec
		spec.Rules = []networkingv1.IngressRule{
			{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &path,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: svc.Name,
										Port: networkingv1.ServiceBackendPort{
											Name: port,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		return nil
	}
}

// EnsureIngressTLS set flags for Openshift cluster to auto-create TLS termination
func EnsureIngressTLS() func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {

		if ingress.Annotations == nil {
			ingress.Annotations = map[string]string{}
		}
		ingress.Annotations["route.openshift.io/termination"] = "edge"

		if ingress.Spec.TLS == nil {
			// ocp is able to autogenerate TLS
			ingress.Spec.TLS = []networkingv1.IngressTLS{
				{},
			}
		}
		return nil
	}
}
