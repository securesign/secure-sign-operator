package utils

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Expose(ctx context.Context, cli client.Client, svcName string, port string) *routev1.Route {

	// TODO
	//return &routev1.Route{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "",
	//		Namespace: "",
	//	},
	//	Spec: routev1.RouteSpec{
	//		To:             routev1.RouteTargetReference{},
	//		Port:           &routev1.RoutePort{TargetPort: intstr.FromString(port)},
	//		TLS:            nil,
	//		WildcardPolicy: "",
	//	},
	//}

	return nil
}
