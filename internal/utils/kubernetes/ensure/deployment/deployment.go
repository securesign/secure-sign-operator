package deployment

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

func Proxy(noProxy ...string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		ensure.SetProxyEnvs(dp.Spec.Template.Spec.Containers, noProxy...)
		return nil
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference, containerName string, moreNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TrustedCA(lor, containerName, moreNames...)(&dp.Spec.Template)
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls v1alpha1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TLS(tls, containerNames...)(&dp.Spec.Template)
	}
}

func PodRequirements(requirements v1alpha1.PodRequirements, containerName string) func(*v1.Deployment) error {
	return func(deployment *v1.Deployment) error {
		deployment.Spec.Replicas = requirements.Replicas

		template := &deployment.Spec.Template
		template.Spec.Affinity = requirements.Affinity
		template.Spec.Tolerations = requirements.Tolerations

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		if requirements.Resources != nil {
			container.Resources = *requirements.Resources
		} else {
			container.Resources = core.ResourceRequirements{}
		}
		return nil
	}
}

func PodSecurityContext() func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensure.PodSecurityContext(&dp.Spec.Template.Spec)
	}
}
