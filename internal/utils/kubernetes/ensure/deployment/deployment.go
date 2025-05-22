package deployment

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
	v1 "k8s.io/api/apps/v1"
)

func Proxy(noProxy ...string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		ensure.SetProxyEnvs(dp.Spec.Template.Spec.Containers, noProxy...)
		return nil
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TrustedCA(lor, containerNames...)(&dp.Spec.Template)
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls v1alpha1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TLS(tls, containerNames...)(&dp.Spec.Template)
	}
}
