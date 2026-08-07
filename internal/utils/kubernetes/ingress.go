package kubernetes

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
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

// haproxyRateLimitConnectionsDisabledValue is the exact, case-sensitive
// annotation value that opts out of HAProxy throttling.
const haproxyRateLimitConnectionsDisabledValue = "false"

// EnsureIngressHAProxyThrottling sets OCP-native HAProxy rate-limiting and
// connection-limit annotations on the Ingress, propagated to the auto-created Route.
// Component defaults are always applied. If the user sets
// "haproxy.router.openshift.io/rate-limit-connections" to "false" in their
// Ingress.Annotations, all HAProxy rate-limiting annotations are removed instead.
// User-provided annotation overrides take effect via a separate ensure.Annotations
// call that runs after this function in the ensure chain.
func EnsureIngressHAProxyThrottling(userAnnotations map[string]string, defaults IngressThrottlingDefaults) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {
		if userAnnotations[annotations.HaproxyRateLimitConnections] == haproxyRateLimitConnectionsDisabledValue {
			for _, key := range haproxyThrottlingAnnotationKeys {
				delete(ingress.Annotations, key)
			}
			return nil
		}

		if ingress.Annotations == nil {
			ingress.Annotations = map[string]string{}
		}
		ingress.Annotations[annotations.HaproxyRateLimitConnections] = annotations.HaproxyRateLimitConnectionsValue
		ingress.Annotations[annotations.HaproxyRateLimitConcurrentTCP] = strconv.FormatInt(int64(defaults.ConcurrentTCP), 10)
		ingress.Annotations[annotations.HaproxyRateLimitRateHTTP] = strconv.FormatInt(int64(defaults.RateHTTP), 10)
		ingress.Annotations[annotations.HaproxyRateLimitRateTCP] = strconv.FormatInt(int64(defaults.RateTCP), 10)
		return nil
	}
}

// EnsureIngressAnnotations reconciles user-provided annotations onto the Ingress.
//
// Unlike ensure.Annotations, whose managed-key list is a fixed set the caller
// passes in, this function's managed set is the user's own CR annotations map —
// which means a key removed from the CR is also absent from any managed-key list
// derived from the CR's *current* state, so a naive caller could never detect the
// removal. Instead, this function persists the key set it applied on the previous
// reconcile in annotations.ManagedIngressAnnotationKeys, so it can delete exactly
// those keys before copying in the current desired set.
//
// IMPORTANT: The HAProxy throttling annotation keys (haproxyThrottlingAnnotationKeys)
// are excluded from tracking. Their full lifecycle (set to defaults, deleted on
// opt-out) is owned by EnsureIngressHAProxyThrottling, which is called earlier
// in the ensure chain and unconditionally recomputes them every reconcile. If we
// tracked them here, a removal from the CR would cause this function to delete
// them after EnsureIngressHAProxyThrottling just re-added them in the same
// reconcile, breaking re-enablement of throttling. User-provided overrides of
// those keys (e.g., concurrentTCP=10) are still copied in normally; they just
// aren't recorded for deletion on removal.
func EnsureIngressAnnotations(userAnnotations map[string]string) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {
		var previouslyManaged []string
		if raw, ok := ingress.Annotations[annotations.ManagedIngressAnnotationKeys]; ok {
			// A missing or corrupt tracking value is treated as an empty managed
			// set rather than failing reconciliation.
			_ = json.Unmarshal([]byte(raw), &previouslyManaged)
		}

		if ingress.Annotations == nil {
			ingress.Annotations = map[string]string{}
		}
		for _, key := range previouslyManaged {
			delete(ingress.Annotations, key)
		}
		maps.Copy(ingress.Annotations, userAnnotations)

		// Compute the set of keys to track, excluding the HAProxy throttling keys
		// which are managed by EnsureIngressHAProxyThrottling.
		trackable := maps.Clone(userAnnotations)
		for _, key := range haproxyThrottlingAnnotationKeys {
			delete(trackable, key)
		}
		nowManaged := slices.Sorted(maps.Keys(trackable))
		if nowManaged == nil {
			nowManaged = []string{}
		}
		encoded, err := json.Marshal(nowManaged)
		if err != nil {
			return err
		}
		ingress.Annotations[annotations.ManagedIngressAnnotationKeys] = string(encoded)
		return nil
	}
}
