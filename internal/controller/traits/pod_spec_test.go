package traits

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/utils/tls"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func podSpecWithContainers(names ...string) *corev1ac.PodSpecApplyConfiguration {
	containers := make([]*corev1ac.ContainerApplyConfiguration, 0, len(names))
	for _, name := range names {
		containers = append(containers, corev1ac.Container().WithName(name))
	}
	return corev1ac.PodSpec().WithContainers(containers...)
}

func envValue(container corev1ac.ContainerApplyConfiguration, name string) (string, bool) {
	for _, env := range container.Env {
		if env.Name != nil && *env.Name == name && env.Value != nil {
			return *env.Value, true
		}
	}
	return "", false
}

func TestAuth(t *testing.T) {
	t.Parallel()

	t.Run("nil auth is a no-op", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(Auth(nil, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Volumes).To(BeEmpty())
		g.Expect(podSpec.Containers[0].Env).To(BeEmpty())
		g.Expect(podSpec.Containers[0].VolumeMounts).To(BeEmpty())
	})

	t.Run("adds auth only to selected containers", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server", "sidecar")
		auth := &rhtasv1.Auth{
			Env: []core.EnvVar{{Name: "AUTH_MODE", Value: "token"}},
			SecretMount: []rhtasv1.SecretKeySelector{
				{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "credentials"}},
				{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secondary"}},
			},
		}

		g.Expect(Auth(auth, "server")(podSpec)).To(Succeed())

		g.Expect(podSpec.Volumes).To(HaveLen(1))
		g.Expect(podSpec.Volumes[0].Name).To(HaveValue(Equal(authVolumeName)))
		g.Expect(podSpec.Volumes[0].Projected).ToNot(BeNil())
		g.Expect(podSpec.Volumes[0].Projected.DefaultMode).To(HaveValue(Equal(core.ProjectedVolumeSourceDefaultMode)))
		g.Expect(podSpec.Volumes[0].Projected.Sources).To(HaveLen(2))
		g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Name).To(HaveValue(Equal("credentials")))
		g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Items).To(BeEmpty())
		g.Expect(podSpec.Volumes[0].Projected.Sources[1].Secret.Name).To(HaveValue(Equal("secondary")))

		value, found := envValue(podSpec.Containers[0], "AUTH_MODE")
		g.Expect(found).To(BeTrue())
		g.Expect(value).To(Equal("token"))
		g.Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(1))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].Name).To(HaveValue(Equal(authVolumeName)))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(HaveValue(Equal(AuthMountPath)))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].ReadOnly).To(HaveValue(BeTrue()))

		g.Expect(podSpec.Containers[1].Env).To(BeEmpty())
		g.Expect(podSpec.Containers[1].VolumeMounts).To(BeEmpty())
	})
}

func TestPodExtensions(t *testing.T) {
	t.Parallel()

	t.Run("adds extensions alongside existing volumes and mounts", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")
		restartPolicy := core.ContainerRestartPolicyAlways
		runAsNonRoot := true
		podSpec.WithVolumes(corev1ac.Volume().WithName("custom").WithEmptyDir(corev1ac.EmptyDirVolumeSource()))
		podSpec.Containers[0].WithVolumeMounts(
			corev1ac.VolumeMount().WithName("custom").WithMountPath("/old"),
		)

		ext := rhtasv1.PodExtensions{
			InitContainers: []rhtasv1.InitContainerSpec{{
				Name:            "setup",
				Image:           "setup:latest",
				Command:         []string{"/bin/setup"},
				Args:            []string{"--config", "/config/settings.yaml"},
				Env:             []core.EnvVar{{Name: "MODE", Value: "initialize"}},
				EnvFrom:         []core.EnvFromSource{{Prefix: "SETUP_", ConfigMapRef: &core.ConfigMapEnvSource{LocalObjectReference: core.LocalObjectReference{Name: "setup-env"}}}},
				VolumeMounts:    []core.VolumeMount{{Name: "custom", MountPath: "/config", ReadOnly: true}},
				Resources:       &core.ResourceRequirements{Requests: core.ResourceList{core.ResourceCPU: resource.MustParse("10m")}},
				SecurityContext: &core.SecurityContext{RunAsNonRoot: &runAsNonRoot},
				ImagePullPolicy: core.PullAlways,
				RestartPolicy:   &restartPolicy,
			}},
			Volumes: []rhtasv1.AdditionalVolume{{
				Name: "settings",
				AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{Name: "settings"},
					},
				},
			}},
			VolumeMounts: []core.VolumeMount{{Name: "settings", MountPath: "/config", ReadOnly: true}},
		}

		g.Expect(PodExtensions(ext, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.InitContainers).To(HaveLen(1))
		g.Expect(podSpec.InitContainers[0].Name).To(HaveValue(Equal("setup")))
		g.Expect(podSpec.InitContainers[0].Image).To(HaveValue(Equal("setup:latest")))
		g.Expect(podSpec.InitContainers[0].Command).To(Equal([]string{"/bin/setup"}))
		g.Expect(podSpec.InitContainers[0].Args).To(Equal([]string{"--config", "/config/settings.yaml"}))
		g.Expect(podSpec.InitContainers[0].Env).To(HaveLen(1))
		g.Expect(podSpec.InitContainers[0].Env[0].Name).To(HaveValue(Equal("MODE")))
		g.Expect(podSpec.InitContainers[0].Env[0].Value).To(HaveValue(Equal("initialize")))
		g.Expect(podSpec.InitContainers[0].EnvFrom).To(HaveLen(1))
		g.Expect(podSpec.InitContainers[0].EnvFrom[0].Prefix).To(HaveValue(Equal("SETUP_")))
		g.Expect(podSpec.InitContainers[0].EnvFrom[0].ConfigMapRef.Name).To(HaveValue(Equal("setup-env")))
		g.Expect(podSpec.InitContainers[0].VolumeMounts).To(HaveLen(1))
		g.Expect(podSpec.InitContainers[0].VolumeMounts[0].Name).To(HaveValue(Equal("custom")))
		g.Expect(podSpec.InitContainers[0].VolumeMounts[0].MountPath).To(HaveValue(Equal("/config")))
		g.Expect(podSpec.InitContainers[0].VolumeMounts[0].ReadOnly).To(HaveValue(BeTrue()))
		g.Expect(podSpec.InitContainers[0].Resources.Requests).ToNot(BeNil())
		g.Expect((*podSpec.InitContainers[0].Resources.Requests)[core.ResourceCPU]).To(Equal(resource.MustParse("10m")))
		g.Expect(podSpec.InitContainers[0].SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
		g.Expect(podSpec.InitContainers[0].ImagePullPolicy).To(HaveValue(Equal(core.PullAlways)))
		g.Expect(podSpec.InitContainers[0].RestartPolicy).To(HaveValue(Equal(core.ContainerRestartPolicyAlways)))
		g.Expect(podSpec.Volumes).To(HaveLen(2))
		g.Expect(podSpec.Volumes[0].Name).To(HaveValue(Equal("custom")))
		g.Expect(podSpec.Volumes[0].ConfigMap).To(BeNil())
		g.Expect(podSpec.Volumes[0].EmptyDir).ToNot(BeNil())
		g.Expect(podSpec.Volumes[1].Name).To(HaveValue(Equal("settings")))
		g.Expect(podSpec.Volumes[1].ConfigMap).ToNot(BeNil())
		g.Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(2))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(HaveValue(Equal("/old")))
		g.Expect(podSpec.Containers[0].VolumeMounts[0].ReadOnly).To(BeNil())
		g.Expect(podSpec.Containers[0].VolumeMounts[1].Name).To(HaveValue(Equal("settings")))
		g.Expect(podSpec.Containers[0].VolumeMounts[1].MountPath).To(HaveValue(Equal("/config")))
		g.Expect(podSpec.Containers[0].VolumeMounts[1].ReadOnly).To(HaveValue(BeTrue()))
	})

	t.Run("returns ErrPodExtensionConflict for duplicate extensions", func(t *testing.T) {
		tests := []struct {
			name           string
			podSpec        *corev1ac.PodSpecApplyConfiguration
			ext            rhtasv1.PodExtensions
			containerNames []string
		}{
			{
				name: "volume",
				podSpec: corev1ac.PodSpec().WithVolumes(
					corev1ac.Volume().WithName("custom"),
				),
				ext: rhtasv1.PodExtensions{Volumes: []rhtasv1.AdditionalVolume{{
					Name: "custom",
					AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
						EmptyDir: &core.EmptyDirVolumeSource{},
					},
				}}},
			},
			{
				name: "volume mount",
				podSpec: func() *corev1ac.PodSpecApplyConfiguration {
					podSpec := podSpecWithContainers("server")
					podSpec.Containers[0].WithVolumeMounts(
						corev1ac.VolumeMount().WithName("custom").WithMountPath("/old"),
					)
					return podSpec
				}(),
				ext: rhtasv1.PodExtensions{VolumeMounts: []core.VolumeMount{{
					Name: "custom", MountPath: "/new",
				}}},
				containerNames: []string{"server"},
			},
			{
				name: "init container",
				podSpec: corev1ac.PodSpec().WithInitContainers(
					corev1ac.Container().WithName("setup").WithImage("operator:latest"),
				),
				ext: rhtasv1.PodExtensions{InitContainers: []rhtasv1.InitContainerSpec{{
					Name:  "setup",
					Image: "user:latest",
				}}},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)

				err := PodExtensions(tt.ext, tt.containerNames...)(tt.podSpec)
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, ErrPodExtensionConflict)).To(BeTrue())
			})
		}
	})
}

func TestProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example")
	t.Setenv("HTTPS_PROXY", "https://proxy.example")
	t.Setenv("NO_PROXY", "cluster.local")

	g := NewWithT(t)
	podSpec := podSpecWithContainers("server", "sidecar")
	g.Expect(Proxy([]string{"server"}, "svc.cluster.local")(podSpec)).To(Succeed())

	value, found := envValue(podSpec.Containers[0], "HTTP_PROXY")
	g.Expect(found).To(BeTrue())
	g.Expect(value).To(Equal("http://proxy.example"))
	value, found = envValue(podSpec.Containers[0], "NO_PROXY")
	g.Expect(found).To(BeTrue())
	g.Expect(value).To(Equal("svc.cluster.local,cluster.local"))
	g.Expect(podSpec.Containers[1].Env).To(BeEmpty())
}

func TestGODEBUG(t *testing.T) {
	t.Run("operator environment", func(t *testing.T) {
		t.Setenv("GODEBUG", "fips140=only")
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server", "sidecar")

		g.Expect(GODEBUG(nil, "server")(podSpec)).To(Succeed())
		value, found := envValue(podSpec.Containers[0], "GODEBUG")
		g.Expect(found).To(BeTrue())
		g.Expect(value).To(Equal("fips140=only"))
		g.Expect(podSpec.Containers[1].Env).To(BeEmpty())
	})

	t.Run("annotation overrides operator environment", func(t *testing.T) {
		t.Setenv("GODEBUG", "fips140=only")
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(GODEBUG(map[string]string{annotations.Godebug: "fips140=on"}, "server")(podSpec)).To(Succeed())
		value, found := envValue(podSpec.Containers[0], "GODEBUG")
		g.Expect(found).To(BeTrue())
		g.Expect(value).To(Equal("fips140=on"))
	})

	t.Run("empty annotation omits desired environment variable", func(t *testing.T) {
		t.Setenv("GODEBUG", "fips140=only")
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(GODEBUG(map[string]string{annotations.Godebug: ""}, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Containers[0].Env).To(BeEmpty())
	})
}

func TestTrustedCA(t *testing.T) {
	t.Run("config map is mounted only into matching containers", func(t *testing.T) {
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server", "sidecar")

		g.Expect(TrustedCA(&rhtasv1.LocalObjectReference{Name: "my-ca"}, "server")(podSpec)).To(Succeed())

		value, found := envValue(podSpec.Containers[0], "SSL_CERT_DIR")
		g.Expect(found).To(BeTrue())
		g.Expect(value).To(Equal(tls.CATrustMountPath + ":/var/run/secrets/kubernetes.io/serviceaccount"))
		g.Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(1))
		g.Expect(*podSpec.Containers[0].VolumeMounts[0].Name).To(Equal(tls.CaTrustVolumeName))
		g.Expect(podSpec.Containers[1].Env).To(BeEmpty())
		g.Expect(podSpec.Containers[1].VolumeMounts).To(BeEmpty())
		g.Expect(podSpec.Volumes).To(HaveLen(1))
		g.Expect(podSpec.Volumes[0].Projected.Sources).To(HaveLen(1))
		g.Expect(*podSpec.Volumes[0].Projected.Sources[0].ConfigMap.Name).To(Equal("my-ca"))
	})

	t.Run("nil reference declares an empty projection with explicit mode", func(t *testing.T) {
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(TrustedCA(nil, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Volumes).To(HaveLen(1))
		g.Expect(podSpec.Volumes[0].EmptyDir).To(BeNil())
		g.Expect(podSpec.Volumes[0].Projected).NotTo(BeNil())
		g.Expect(podSpec.Volumes[0].Projected.Sources).To(BeEmpty())
		g.Expect(podSpec.Volumes[0].Projected.DefaultMode).To(HaveValue(Equal(core.ProjectedVolumeSourceDefaultMode)))
	})

	t.Run("no matching container skips the volume", func(t *testing.T) {
		g := NewWithT(t)
		podSpec := podSpecWithContainers("sidecar")

		g.Expect(TrustedCA(&rhtasv1.LocalObjectReference{Name: "my-ca"}, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Volumes).To(BeEmpty())
	})
}

func TestTLS(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	podSpec := podSpecWithContainers("server", "sidecar")
	tlsConfig := rhtasv1.TLS{
		CertRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tls-certificate"},
			Key:                  "certificate",
		},
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tls-private-key"},
			Key:                  "private-key",
		},
	}

	g.Expect(TLS(tlsConfig, "server")(podSpec)).To(Succeed())

	g.Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(podSpec.Containers[0].VolumeMounts[0].Name).To(HaveValue(Equal(tls.TLSVolumeName)))
	g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(HaveValue(Equal(tls.TLSVolumeMount)))
	g.Expect(podSpec.Containers[0].VolumeMounts[0].ReadOnly).To(HaveValue(BeTrue()))
	g.Expect(podSpec.Containers[1].VolumeMounts).To(BeEmpty())

	g.Expect(podSpec.Volumes).To(HaveLen(1))
	g.Expect(podSpec.Volumes[0].Name).To(HaveValue(Equal(tls.TLSVolumeName)))
	g.Expect(podSpec.Volumes[0].Projected).ToNot(BeNil())
	g.Expect(podSpec.Volumes[0].Projected.Sources).To(HaveLen(2))
	g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Name).To(HaveValue(Equal("tls-certificate")))
	g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Items).To(HaveLen(1))
	g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Items[0].Key).To(HaveValue(Equal("certificate")))
	g.Expect(podSpec.Volumes[0].Projected.Sources[0].Secret.Items[0].Path).To(HaveValue(Equal("tls.crt")))
	g.Expect(podSpec.Volumes[0].Projected.Sources[1].Secret.Name).To(HaveValue(Equal("tls-private-key")))
	g.Expect(podSpec.Volumes[0].Projected.Sources[1].Secret.Items).To(HaveLen(1))
	g.Expect(podSpec.Volumes[0].Projected.Sources[1].Secret.Items[0].Key).To(HaveValue(Equal("private-key")))
	g.Expect(podSpec.Volumes[0].Projected.Sources[1].Secret.Items[0].Path).To(HaveValue(Equal("tls.key")))
}

func TestSecurityContexts(t *testing.T) {
	originalOpenShift := config.Openshift
	t.Cleanup(func() { config.Openshift = originalOpenShift })

	t.Run("Kubernetes defaults", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server", "sidecar")

		g.Expect(SecurityContexts("server")(podSpec)).To(Succeed())
		g.Expect(podSpec.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
		g.Expect(podSpec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(1001))))
		g.Expect(*podSpec.SecurityContext.SeccompProfile.Type).To(Equal(core.SeccompProfileTypeRuntimeDefault))
		g.Expect(podSpec.Containers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))))
		g.Expect(podSpec.Containers[0].SecurityContext.AllowPrivilegeEscalation).To(HaveValue(BeFalse()))
		g.Expect(podSpec.Containers[0].SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")))
		g.Expect(podSpec.Containers[1].SecurityContext).To(BeNil())
	})

	t.Run("OpenShift omits fixed IDs", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(SecurityContexts("server")(podSpec)).To(Succeed())
		g.Expect(podSpec.SecurityContext.FSGroup).To(BeNil())
		g.Expect(podSpec.Containers[0].SecurityContext.RunAsUser).To(BeNil())
	})
}

func TestServiceAccount(t *testing.T) {
	g := NewWithT(t)
	podSpec := corev1ac.PodSpec()
	g.Expect(ServiceAccount("tuf")(podSpec)).To(Succeed())
	g.Expect(podSpec.ServiceAccountName).To(HaveValue(Equal("tuf")))
}
