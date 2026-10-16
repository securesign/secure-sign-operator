package traits

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/utils/tls"
	core "k8s.io/api/core/v1"
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

	t.Run("nil reference declares an emptyDir volume", func(t *testing.T) {
		g := NewWithT(t)
		podSpec := podSpecWithContainers("server")

		g.Expect(TrustedCA(nil, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Volumes).To(HaveLen(1))
		g.Expect(podSpec.Volumes[0].EmptyDir).ToNot(BeNil())
		g.Expect(podSpec.Volumes[0].Projected).To(BeNil())
	})

	t.Run("no matching container skips the volume", func(t *testing.T) {
		g := NewWithT(t)
		podSpec := podSpecWithContainers("sidecar")

		g.Expect(TrustedCA(&rhtasv1.LocalObjectReference{Name: "my-ca"}, "server")(podSpec)).To(Succeed())
		g.Expect(podSpec.Volumes).To(BeEmpty())
	})
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
