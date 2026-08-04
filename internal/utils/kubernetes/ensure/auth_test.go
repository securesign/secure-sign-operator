package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	core "k8s.io/api/core/v1"
)

func TestContainerAuth_NilAuth(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{
		Containers: []core.Container{*container},
		Volumes: []core.Volume{
			{Name: authVolumeName, VolumeSource: core.VolumeSource{Projected: &core.ProjectedVolumeSource{}}},
		},
	}
	container = &spec.Containers[0]
	container.VolumeMounts = []core.VolumeMount{
		{Name: authVolumeName, MountPath: AuthMountPath},
	}

	err := ContainerAuth(container, nil)(spec)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(spec.Volumes).To(BeEmpty())
	g.Expect(container.VolumeMounts).To(BeEmpty())
}

func TestContainerAuth_EmptyAuth(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &spec.Containers[0]

	err := ContainerAuth(container, &rhtasv1.Auth{})(spec)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(spec.Volumes).To(BeEmpty())
	g.Expect(container.VolumeMounts).To(BeEmpty())
}

func TestContainerAuth_EnvOnly(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &spec.Containers[0]

	auth := &rhtasv1.Auth{
		Env: []core.EnvVar{
			{Name: "VENDOR_TOKEN", Value: "test-value"},
		},
	}

	err := ContainerAuth(container, auth)(spec)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("VENDOR_TOKEN"))
	g.Expect(container.Env[0].Value).To(Equal("test-value"))

	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Name).To(Equal(authVolumeName))
	g.Expect(spec.Volumes[0].Projected).ToNot(BeNil())
	g.Expect(spec.Volumes[0].Projected.DefaultMode).ToNot(BeNil())

	g.Expect(container.VolumeMounts).To(HaveLen(1))
	g.Expect(container.VolumeMounts[0].Name).To(Equal(authVolumeName))
	g.Expect(container.VolumeMounts[0].MountPath).To(Equal(AuthMountPath))
	g.Expect(container.VolumeMounts[0].ReadOnly).To(BeTrue())
}

func TestContainerAuth_SecretMountOnly(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &spec.Containers[0]

	auth := &rhtasv1.Auth{
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "vendor-creds"}, Key: "token"},
		},
	}

	err := ContainerAuth(container, auth)(spec)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(spec.Volumes).To(HaveLen(1))
	vol := spec.Volumes[0]
	g.Expect(vol.Name).To(Equal(authVolumeName))
	g.Expect(vol.Projected).ToNot(BeNil())
	g.Expect(vol.Projected.Sources).To(HaveLen(1))
	g.Expect(vol.Projected.Sources[0].Secret.Name).To(Equal("vendor-creds"))
}

func TestContainerAuth_EnvAndSecretMount(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &spec.Containers[0]

	auth := &rhtasv1.Auth{
		Env: []core.EnvVar{
			{Name: "TOKEN_A", Value: "a"},
			{Name: "TOKEN_B", Value: "b"},
		},
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret-a"}, Key: "key"},
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret-b"}, Key: "key"},
		},
	}

	err := ContainerAuth(container, auth)(spec)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(container.Env).To(HaveLen(2))
	g.Expect(container.Env[0].Name).To(Equal("TOKEN_A"))
	g.Expect(container.Env[1].Name).To(Equal("TOKEN_B"))

	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Projected.Sources).To(HaveLen(2))
	g.Expect(spec.Volumes[0].Projected.Sources[0].Secret.Name).To(Equal("secret-a"))
	g.Expect(spec.Volumes[0].Projected.Sources[1].Secret.Name).To(Equal("secret-b"))
}

func TestContainerAuth_Idempotent(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &spec.Containers[0]

	auth := &rhtasv1.Auth{
		Env: []core.EnvVar{
			{Name: "VENDOR_TOKEN", Value: "v1"},
		},
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "creds"}, Key: "key"},
		},
	}

	g.Expect(ContainerAuth(container, auth)(spec)).To(Succeed())
	g.Expect(ContainerAuth(container, auth)(spec)).To(Succeed())

	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Projected.Sources).To(HaveLen(1))
	g.Expect(container.VolumeMounts).To(HaveLen(1))
}

func TestContainerAuth_LegacyVolumeCleanup(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{
		Containers: []core.Container{*container},
		Volumes: []core.Volume{
			{Name: legacyAuthVolumeName, VolumeSource: core.VolumeSource{Projected: &core.ProjectedVolumeSource{}}},
		},
	}
	container = &spec.Containers[0]
	container.VolumeMounts = []core.VolumeMount{
		{Name: legacyAuthVolumeName, MountPath: AuthMountPath},
	}

	auth := &rhtasv1.Auth{
		Env: []core.EnvVar{
			{Name: "TOKEN", Value: "val"},
		},
	}

	err := ContainerAuth(container, auth)(spec)
	g.Expect(err).ToNot(HaveOccurred())

	// Legacy volume removed
	for _, v := range spec.Volumes {
		g.Expect(v.Name).ToNot(Equal(legacyAuthVolumeName))
	}
	for _, m := range container.VolumeMounts {
		g.Expect(m.Name).ToNot(Equal(legacyAuthVolumeName))
	}

	// New volume created
	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Name).To(Equal(authVolumeName))
}

func TestContainerAuth_LegacyVolumeCleanupOnNilAuth(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	spec := &core.PodSpec{
		Containers: []core.Container{*container},
		Volumes: []core.Volume{
			{Name: legacyAuthVolumeName, VolumeSource: core.VolumeSource{Projected: &core.ProjectedVolumeSource{}}},
		},
	}
	container = &spec.Containers[0]
	container.VolumeMounts = []core.VolumeMount{
		{Name: legacyAuthVolumeName, MountPath: AuthMountPath},
	}

	err := ContainerAuth(container, nil)(spec)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(spec.Volumes).To(BeEmpty())
	g.Expect(container.VolumeMounts).To(BeEmpty())
}

func TestAuth_ByContainerName(t *testing.T) {
	g := NewWithT(t)
	spec := &core.PodSpec{
		Containers: []core.Container{{Name: "server"}},
	}

	auth := &rhtasv1.Auth{
		Env: []core.EnvVar{
			{Name: "TOKEN", Value: "val"},
		},
	}

	err := Auth("server", auth)(spec)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(spec.Containers[0].Env).To(HaveLen(1))
	g.Expect(spec.Containers[0].Env[0].Name).To(Equal("TOKEN"))
}
