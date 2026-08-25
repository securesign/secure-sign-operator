package kubernetes

import (
	"context"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	v12 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return EnsureIngressTermination("edge")
}

// EnsureIngressTermination sets the given OpenShift Route termination mode
// (e.g. "edge", "reencrypt") for the auto-generated Ingress-to-Route conversion.
func EnsureIngressTermination(termination string) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {

		if ingress.Annotations == nil {
			ingress.Annotations = map[string]string{}
		}
		ingress.Annotations["route.openshift.io/termination"] = termination

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
// User-provided annotation overrides are applied separately via SSA
// (ApplyIngressUserMetadata) after CreateOrUpdate.
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

// ingressUserMetadataApplyConfig is a minimal SSA apply configuration for
// Ingress metadata. It intentionally omits the json:"omitempty" tag on
// Annotations and Labels so that an empty map serializes as "{}" rather than
// being dropped — SSA interprets the empty map as "release all previously-owned
// keys", which is required when a user removes all annotations or labels.
type ingressUserMetadataApplyConfig struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        ingressUserMetadataObjectMeta `json:"metadata"`
}

func (c *ingressUserMetadataApplyConfig) IsApplyConfiguration()  {}
func (c *ingressUserMetadataApplyConfig) GetName() *string       { return &c.Metadata.Name }
func (c *ingressUserMetadataApplyConfig) GetNamespace() *string  { return &c.Metadata.Namespace }
func (c *ingressUserMetadataApplyConfig) GetKind() *string       { return &c.Kind }
func (c *ingressUserMetadataApplyConfig) GetAPIVersion() *string { return &c.APIVersion }

type ingressUserMetadataObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
}

const userMetadataFieldOwner = client.FieldOwner("securesign-user-metadata")

// ApplyIngressUserMetadata uses Server-Side Apply to reconcile user-provided
// annotations and labels onto a managed Ingress. SSA tracks field ownership
// per-key: keys removed from the CR are automatically deleted from the Ingress
// when they are no longer included in the apply configuration.
//
// This alone is not enough to get user labels onto an OpenShift Route: the
// route-controller-manager snapshots Ingress labels onto the auto-created
// Route only once, at Route creation time, and does not re-sync them on later
// Ingress updates. Callers must also merge user labels into the Ingress
// object as part of the same CreateOrUpdate call that first gives it a spec
// (see the ensure.Labels call in each ingress action) so they are already
// present when the Route gets created; this SSA call remains responsible for
// cleaning up labels/annotations the user later removes from the CR.
func ApplyIngressUserMetadata(ctx context.Context, cli client.Client, name, namespace string, userAnnotations, userLabels map[string]string) error {
	if userAnnotations == nil {
		userAnnotations = map[string]string{}
	}
	if userLabels == nil {
		userLabels = map[string]string{}
	}

	config := &ingressUserMetadataApplyConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		Metadata: ingressUserMetadataObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: userAnnotations,
			Labels:      userLabels,
		},
	}
	return cli.Apply(ctx, config, userMetadataFieldOwner, client.ForceOwnership)
}
