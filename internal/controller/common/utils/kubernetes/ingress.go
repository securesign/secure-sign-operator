package kubernetes

import (
	"context"

	"github.com/securesign/operator/api/v1alpha1"
	v12 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateIngress(ctx context.Context, cli client.Client, svc v12.Service, conf v1alpha1.ExternalAccess, port string, labels map[string]string) (*networkingv1.Ingress, error) {
	path := networkingv1.PathTypePrefix
	host := conf.Host
	var tlsConfig []networkingv1.IngressTLS
	var annotations map[string]string

	if IsOpenShift() {
		annotations = map[string]string{"route.openshift.io/termination": "edge"}
		// ocp is able to autogenerate TLS
		tlsConfig = []networkingv1.IngressTLS{
			{},
		}
	}

	if host == "" {
		var err error
		if host, err = CalculateHostname(ctx, cli, svc.Name, svc.Namespace); err != nil {
			return nil, err
		}
	}
	return &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
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
			},
			TLS: tlsConfig,
		},
	}, nil
}
