package traits

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IngressTerminationEdge        = "edge"
	IngressTerminationReencrypt   = "reencrypt"
	IngressTerminationPassthrough = "passthrough"
)

func WithRule(configurations ...Trait[*networkingv1ac.IngressRuleApplyConfiguration]) Trait[*networkingv1ac.IngressApplyConfiguration] {
	return func(ingress *networkingv1ac.IngressApplyConfiguration) error {
		if ingress.Spec == nil {
			ingress.WithSpec(networkingv1ac.IngressSpec())
		}
		rule, err := Apply(networkingv1ac.IngressRule(), configurations...)
		if err != nil {
			return err
		}
		ingress.Spec.WithRules(rule)
		return nil
	}
}

func IngressHost(ctx context.Context, cli client.Client, conf rhtasv1.Ingress, svc corev1.Service) Trait[*networkingv1ac.IngressRuleApplyConfiguration] {
	return func(rule *networkingv1ac.IngressRuleApplyConfiguration) error {
		host := conf.Host
		if host == "" {
			var err error
			if host, err = kubernetes.CalculateHostname(ctx, cli, svc.Name, svc.Namespace); err != nil {
				return err
			}
		}

		rule.WithHost(host)
		return nil
	}
}

func IngressPath(svc corev1.Service, port, path string) Trait[*networkingv1ac.IngressRuleApplyConfiguration] {
	return func(rule *networkingv1ac.IngressRuleApplyConfiguration) error {
		if rule.HTTP == nil {
			rule.WithHTTP(networkingv1ac.HTTPIngressRuleValue())
		}
		rule.HTTP.WithPaths(networkingv1ac.HTTPIngressPath().
			WithPath(path).
			WithPathType(networkingv1.PathTypePrefix).
			WithBackend(networkingv1ac.IngressBackend().
				WithService(networkingv1ac.IngressServiceBackend().
					WithName(svc.Name).
					WithPort(networkingv1ac.ServiceBackendPort().WithName(port)))))
		return nil
	}
}

// IngressTLSWithTermination configures OpenShift's automatic TLS termination mode.
func IngressTLSWithTermination(termination string) Trait[*networkingv1ac.IngressApplyConfiguration] {
	return func(ingress *networkingv1ac.IngressApplyConfiguration) error {
		ingress.WithAnnotations(map[string]string{annotations.IngressTermination: termination})
		return nil
	}
}
