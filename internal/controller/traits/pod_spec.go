package traits

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/operator-framework/operator-lib/proxy"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/utils/tls"
	corev1 "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const (
	authVolumeName = "auth"
	AuthMountPath  = constants.SecretMountPath + "/auth"
)

// Auth adds authentication environment variables and mounts referenced Secrets
// into the selected containers through a shared projected volume.
func Auth(auth *rhtasv1.Auth, containerNames ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if auth == nil {
			return nil
		}

		projected := corev1ac.ProjectedVolumeSource()
		for _, secret := range auth.SecretMount {
			projected.WithSources(
				corev1ac.VolumeProjection().WithSecret(
					corev1ac.SecretProjection().WithName(secret.Name),
				),
			)
		}

		podSpec.WithVolumes(
			corev1ac.Volume().
				WithName(authVolumeName).
				WithProjected(projected),
		)

		for i := range podSpec.Containers {
			container := &podSpec.Containers[i]
			if container.Name == nil || !slices.Contains(containerNames, *container.Name) {
				continue
			}

			for _, env := range auth.Env {
				envc, err := ConvertViaJSON[
					corev1.EnvVar,
					corev1ac.EnvVarApplyConfiguration,
				](env)
				if err != nil {
					return fmt.Errorf("convert env var: %w", err)
				}
				container.WithEnv(envc)
			}

			container.WithVolumeMounts(
				corev1ac.VolumeMount().
					WithName(authVolumeName).
					WithMountPath(AuthMountPath).
					WithReadOnly(true),
			)
		}

		return nil
	}
}

// PodExtensions applies user-defined init containers, volumes, and volume
// mounts to a pod spec apply configuration. Existing entries are preserved by
// name so operator-managed values take precedence.
func PodExtensions(ext rhtasv1.PodExtensions, containerNames ...string) Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpecApply *corev1ac.PodSpecApplyConfiguration) error {
		for _, v := range ext.Volumes {
			if slices.ContainsFunc(podSpecApply.Volumes, func(volume corev1ac.VolumeApplyConfiguration) bool {
				return volume.Name != nil && *volume.Name == v.Name
			}) {
				continue
			}

			vac, err := ConvertViaJSON[corev1.Volume, corev1ac.VolumeApplyConfiguration](v.ToVolume())
			if err != nil {
				return fmt.Errorf("convert volume %q: %w", v.Name, err)
			}
			podSpecApply.WithVolumes(vac)
		}

		// volume mounts
		volumeMounts := make([]*corev1ac.VolumeMountApplyConfiguration, len(ext.VolumeMounts))
		for i, vm := range ext.VolumeMounts {
			vmc, err := ConvertViaJSON[corev1.VolumeMount, corev1ac.VolumeMountApplyConfiguration](vm)
			if err != nil {
				return fmt.Errorf("convert volume mount: %w", err)
			}
			volumeMounts[i] = vmc
		}

		for i := range podSpecApply.Containers {
			if podSpecApply.Containers[i].Name == nil || !slices.Contains(containerNames, *podSpecApply.Containers[i].Name) {
				continue
			}
			container := &podSpecApply.Containers[i]
			for _, volumeMount := range volumeMounts {
				if slices.ContainsFunc(container.VolumeMounts, func(existing corev1ac.VolumeMountApplyConfiguration) bool {
					return existing.Name != nil && volumeMount.Name != nil && *existing.Name == *volumeMount.Name
				}) {
					continue
				}
				container.WithVolumeMounts(volumeMount)
			}
		}

		for _, ic := range ext.InitContainers {
			if slices.ContainsFunc(podSpecApply.InitContainers, func(container corev1ac.ContainerApplyConfiguration) bool {
				return container.Name != nil && *container.Name == ic.Name
			}) {
				continue
			}

			icc, err := ConvertViaJSON[corev1.Container, corev1ac.ContainerApplyConfiguration](ic.ToContainer())
			if err != nil {
				return fmt.Errorf("convert init container: %w", err)
			}
			if icc.SecurityContext == nil {
				icc.WithSecurityContext(SSAContainerSecurityContext())
			}
			podSpecApply.WithInitContainers(icc)
		}
		return nil
	}
}

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
