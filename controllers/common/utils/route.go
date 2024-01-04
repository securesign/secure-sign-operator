package utils

import (
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateRoute(svc v1.Service, port string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: svc.Kind,
				Name: svc.Name,
			},
			Port: &routev1.RoutePort{TargetPort: intstr.FromString(port)},
			TLS: &routev1.TLSConfig{
				Termination: "edge",
			},
			WildcardPolicy: "None",
		},
	}
}
