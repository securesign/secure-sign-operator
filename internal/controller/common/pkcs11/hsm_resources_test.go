package pkcs11

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	core "k8s.io/api/core/v1"
)

func TestEnsureHSMResources_DefaultEmptyDir(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	podSpec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &podSpec.Containers[0]

	EnsureHSMResources(podSpec, container, nil)

	// Volumes created
	g.Expect(podSpec.Volumes).To(HaveLen(2))
	var tokens, lib *core.Volume
	for i := range podSpec.Volumes {
		switch podSpec.Volumes[i].Name {
		case constants.HSMTokensVolumeName:
			tokens = &podSpec.Volumes[i]
		case constants.HSMLibVolumeName:
			lib = &podSpec.Volumes[i]
		}
	}
	g.Expect(tokens).ToNot(BeNil())
	g.Expect(tokens.EmptyDir).ToNot(BeNil())
	g.Expect(lib).ToNot(BeNil())
	g.Expect(lib.EmptyDir).ToNot(BeNil())

	// Mounts created
	g.Expect(container.VolumeMounts).To(HaveLen(2))
	var tokenMount, libMount *core.VolumeMount
	for i := range container.VolumeMounts {
		switch container.VolumeMounts[i].Name {
		case constants.HSMTokensVolumeName:
			tokenMount = &container.VolumeMounts[i]
		case constants.HSMLibVolumeName:
			libMount = &container.VolumeMounts[i]
		}
	}
	g.Expect(tokenMount).ToNot(BeNil())
	g.Expect(tokenMount.MountPath).To(Equal(constants.HSMTokensMountPath))
	g.Expect(libMount).ToNot(BeNil())
	g.Expect(libMount.MountPath).To(Equal(constants.HSMLibMountPath))
	g.Expect(libMount.ReadOnly).To(BeTrue())
}

func TestEnsureHSMResources_UserPVCPreserved(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	podSpec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &podSpec.Containers[0]

	userVolumes := []rhtasv1.AdditionalVolume{
		{
			Name: constants.HSMTokensVolumeName,
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: "my-pvc",
				},
			},
		},
	}

	// Pre-populate the volume on the podSpec (simulates PodResources running first)
	podSpec.Volumes = append(podSpec.Volumes, userVolumes[0].ToVolume())

	EnsureHSMResources(podSpec, container, userVolumes)

	// hsm-tokens should keep PVC source
	var tokens *core.Volume
	for i := range podSpec.Volumes {
		if podSpec.Volumes[i].Name == constants.HSMTokensVolumeName {
			tokens = &podSpec.Volumes[i]
		}
	}
	g.Expect(tokens).ToNot(BeNil())
	g.Expect(tokens.PersistentVolumeClaim).ToNot(BeNil())
	g.Expect(tokens.PersistentVolumeClaim.ClaimName).To(Equal("my-pvc"))
	g.Expect(tokens.EmptyDir).To(BeNil())
}

func TestCleanupHSMResources(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{
		Name: "server",
		VolumeMounts: []core.VolumeMount{
			{Name: constants.HSMTokensVolumeName, MountPath: constants.HSMTokensMountPath},
			{Name: constants.HSMLibVolumeName, MountPath: constants.HSMLibMountPath},
			{Name: "other", MountPath: "/other"},
		},
	}
	podSpec := &core.PodSpec{
		Containers: []core.Container{*container},
		Volumes: []core.Volume{
			{Name: constants.HSMTokensVolumeName},
			{Name: constants.HSMLibVolumeName},
			{Name: "other"},
		},
	}
	container = &podSpec.Containers[0]

	CleanupHSMResources(podSpec, container)

	g.Expect(podSpec.Volumes).To(HaveLen(1))
	g.Expect(podSpec.Volumes[0].Name).To(Equal("other"))
	g.Expect(container.VolumeMounts).To(HaveLen(1))
	g.Expect(container.VolumeMounts[0].Name).To(Equal("other"))
}

func TestEnsureHSMResources_Idempotent(t *testing.T) {
	g := NewWithT(t)
	container := &core.Container{Name: "server"}
	podSpec := &core.PodSpec{Containers: []core.Container{*container}}
	container = &podSpec.Containers[0]

	EnsureHSMResources(podSpec, container, nil)
	EnsureHSMResources(podSpec, container, nil)

	g.Expect(podSpec.Volumes).To(HaveLen(2))
	g.Expect(container.VolumeMounts).To(HaveLen(2))
}
