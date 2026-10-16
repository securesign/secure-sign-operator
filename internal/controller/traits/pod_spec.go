package traits

import (
	"os"
	"slices"
	"strings"

	"github.com/operator-framework/operator-lib/proxy"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/tls"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// Proxy appends HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables to
// matching containers. It is a no-op when no proxy is configured.
func Proxy(containerNames []string, noProxy ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		envs := proxy.ReadProxyVarsFromEnv()
		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name != nil && slices.Contains(containerNames, *podSpec.Containers[i].Name) {
				for _, e := range envs {
					value := e.Value
					if strings.EqualFold(e.Name, "no_proxy") && len(noProxy) > 0 {
						value = strings.Join(noProxy, ",") + "," + value
					}
					podSpec.Containers[i].WithEnv(corev1ac.EnvVar().WithName(e.Name).WithValue(value))
				}
			}
		}
		return nil
	}
}

// GODEBUG appends the GODEBUG env var to matching containers. The value
// is resolved from the component annotation or the operator's own env var.
// No-op when neither is set or annotation is explicitly empty (opt-out).
func GODEBUG(componentAnnotations map[string]string, containerNames ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		annotationValue, annotationOk := componentAnnotations[annotations.Godebug]
		osValue := os.Getenv("GODEBUG")

		var value string
		switch {
		case annotationOk && annotationValue != "":
			value = annotationValue
		case !annotationOk && osValue != "":
			value = osValue
		default:
			return nil
		}

		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name != nil && slices.Contains(containerNames, *podSpec.Containers[i].Name) {
				podSpec.Containers[i].WithEnv(corev1ac.EnvVar().WithName("GODEBUG").WithValue(value))
			}
		}
		return nil
	}
}

// TrustedCA appends the SSL_CERT_DIR env var and CA volume mount to
// matching containers, and the projected CA volume to the pod spec. The
// env var and mount are always added; the config map projection is only
// included when lor is non-nil.
func TrustedCA(lor *rhtasv1.LocalObjectReference, containerNames ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		matched := false
		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name != nil && slices.Contains(containerNames, *podSpec.Containers[i].Name) {
				podSpec.Containers[i].WithEnv(corev1ac.EnvVar().
					WithName("SSL_CERT_DIR").
					WithValue(tls.CATrustMountPath + ":/var/run/secrets/kubernetes.io/serviceaccount"))
				podSpec.Containers[i].WithVolumeMounts(corev1ac.VolumeMount().
					WithName(tls.CaTrustVolumeName).
					WithMountPath(tls.CATrustMountPath).
					WithReadOnly(true))
				matched = true
			}
		}
		if !matched {
			return nil
		}

		volume := corev1ac.Volume().
			WithName(tls.CaTrustVolumeName)

		if lor == nil {
			// Specify the volume explicitly; otherwise emptyDir is defaulted and conflicts with the SSA owner.
			volume.WithEmptyDir(corev1ac.EmptyDirVolumeSource())
		} else {
			volume.WithProjected(corev1ac.ProjectedVolumeSource().
				WithSources(corev1ac.VolumeProjection().
					WithConfigMap(corev1ac.ConfigMapProjection().
						WithName(lor.Name))))
		}

		podSpec.WithVolumes(volume)
		return nil
	}
}

// SecurityContexts applies the standard pod security context and container
// security context to matching containers.
func SecurityContexts(containerNames ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		podSpec.WithSecurityContext(SSAPodSecurityContext())
		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name != nil && slices.Contains(containerNames, *podSpec.Containers[i].Name) {
				podSpec.Containers[i].WithSecurityContext(SSAContainerSecurityContext())
			}
		}
		return nil
	}
}

func ServiceAccount(name string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		podSpec.WithServiceAccountName(name)
		return nil
	}
}
