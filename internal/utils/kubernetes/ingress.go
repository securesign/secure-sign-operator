package kubernetes

import (
	"context"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	v12 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
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

// IngressThrottlingDefaults provides component-specific fallback values for
// HAProxy rate-limiting when the CR does not override them.
type IngressThrottlingDefaults struct {
	ConcurrentTCP int32
	RateHTTP      int32
	RateTCP       int32
}

// haproxyThrottlingAnnotationKeys lists the annotation keys managed by
// EnsureIngressHAProxyThrottling, used to remove them when throttling is disabled.
var haproxyThrottlingAnnotationKeys = []string{
	annotations.HaproxyRateLimitConnections,
	annotations.HaproxyRateLimitConcurrentTCP,
	annotations.HaproxyRateLimitRateHTTP,
	annotations.HaproxyRateLimitRateTCP,
}

// EnsureIngressHAProxyThrottling sets OCP-native HAProxy rate-limiting and
// connection-limit annotations on the Ingress, propagated to the auto-created Route.
// If throttling is nil, the component defaults are applied. If throttling.Enabled
// is explicitly false, any previously-set throttling annotations are removed.
func EnsureIngressHAProxyThrottling(throttling *rhtasv1.IngressThrottling, defaults IngressThrottlingDefaults) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {
		if throttling != nil && !ptr.Deref(throttling.Enabled, true) {
			for _, key := range haproxyThrottlingAnnotationKeys {
				delete(ingress.Annotations, key)
			}
			return nil
		}

		concurrentTCP := defaults.ConcurrentTCP
		rateHTTP := defaults.RateHTTP
		rateTCP := defaults.RateTCP
		if throttling != nil {
			if throttling.ConcurrentTCP != nil {
				concurrentTCP = *throttling.ConcurrentTCP
			}
			if throttling.RateHTTP != nil {
				rateHTTP = *throttling.RateHTTP
			}
			if throttling.RateTCP != nil {
				rateTCP = *throttling.RateTCP
			}
		}

		if ingress.Annotations == nil {
			ingress.Annotations = map[string]string{}
		}
		ingress.Annotations[annotations.HaproxyRateLimitConnections] = annotations.HaproxyRateLimitConnectionsValue
		ingress.Annotations[annotations.HaproxyRateLimitConcurrentTCP] = strconv.FormatInt(int64(concurrentTCP), 10)
		ingress.Annotations[annotations.HaproxyRateLimitRateHTTP] = strconv.FormatInt(int64(rateHTTP), 10)
		ingress.Annotations[annotations.HaproxyRateLimitRateTCP] = strconv.FormatInt(int64(rateTCP), 10)
		return nil
	}
}
