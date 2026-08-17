package deployment

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
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

func GODEBUG(componentAnnotations map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensure.GODEBUG(componentAnnotations)(&dp.Spec.Template.Spec)
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *rhtasv1.LocalObjectReference, containerName string, moreNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TrustedCA(lor, containerName, moreNames...)(&dp.Spec.Template)
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls rhtasv1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return tlsensure.TLS(tls, containerNames...)(&dp.Spec.Template)
	}
}

func PodRequirements(requirements rhtasv1.PodRequirements, containerName string) func(*v1.Deployment) error {
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

// PodExtensions applies user-defined init containers, volumes, and volume mounts
// to a Deployment. It reads the previous state from the tracking annotation,
// removes stale resources, upserts desired ones, and writes the current state back.
func PodExtensions(ext rhtasv1.PodExtensions, containerName string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		prev := ensure.ReadLastApplied(dp)
		template := &dp.Spec.Template
		current, err := ensure.ReconcileUserPodResources(&template.Spec, containerName, ext, prev)
		if err != nil {
			return err
		}
		current.Annotations = prev.Annotations
		current.Labels = prev.Labels
		ensure.WriteLastApplied(dp, current)
		return nil
	}
}

func PodSecurityContext() func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensure.PodSecurityContext(&dp.Spec.Template.Spec)
	}
}

func Auth(containerName string, auth *rhtasv1.Auth) func(*v1.Deployment) error {
	return func(object *v1.Deployment) error {
		return ensure.Auth(containerName, auth)(&object.Spec.Template.Spec)
	}
}
