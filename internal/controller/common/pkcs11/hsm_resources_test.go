package pkcs11

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func TestApplyHSMResources_DefaultEmptyDir(t *testing.T) {
	g := NewWithT(t)
	podSpec := corev1ac.PodSpec().
		WithContainers(corev1ac.Container().WithName("server"))
	container := &podSpec.Containers[0]

	ApplyHSMResources(podSpec, container, nil)

	g.Expect(podSpec.Volumes).To(HaveLen(2))
	g.Expect(podSpec.Volumes[0].Name).To(HaveValue(Equal(constants.HSMLibVolumeName)))
	g.Expect(podSpec.Volumes[0].EmptyDir).ToNot(BeNil())
	g.Expect(podSpec.Volumes[1].Name).To(HaveValue(Equal(constants.HSMTokensVolumeName)))
	g.Expect(podSpec.Volumes[1].EmptyDir).ToNot(BeNil())
	g.Expect(container.VolumeMounts).To(HaveLen(2))
	g.Expect(container.VolumeMounts[0].Name).To(HaveValue(Equal(constants.HSMTokensVolumeName)))
	g.Expect(container.VolumeMounts[0].MountPath).To(HaveValue(Equal(constants.HSMTokensMountPath)))
	g.Expect(container.VolumeMounts[1].Name).To(HaveValue(Equal(constants.HSMLibVolumeName)))
	g.Expect(container.VolumeMounts[1].MountPath).To(HaveValue(Equal(constants.HSMLibMountPath)))
	g.Expect(container.VolumeMounts[1].ReadOnly).To(HaveValue(BeTrue()))
}

func TestApplyHSMResources_UserPVCPreserved(t *testing.T) {
	g := NewWithT(t)
	podSpec := corev1ac.PodSpec().
		WithContainers(corev1ac.Container().WithName("server")).
		WithVolumes(corev1ac.Volume().
			WithName(constants.HSMTokensVolumeName).
			WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().WithClaimName("my-pvc")))
	container := &podSpec.Containers[0]
	userVolumes := []rhtasv1.AdditionalVolume{{Name: constants.HSMTokensVolumeName}}

	ApplyHSMResources(podSpec, container, userVolumes)

	g.Expect(podSpec.Volumes).To(HaveLen(2))
	g.Expect(podSpec.Volumes[0].Name).To(HaveValue(Equal(constants.HSMTokensVolumeName)))
	g.Expect(podSpec.Volumes[0].PersistentVolumeClaim.ClaimName).To(HaveValue(Equal("my-pvc")))
	g.Expect(podSpec.Volumes[0].EmptyDir).To(BeNil())
	g.Expect(podSpec.Volumes[1].Name).To(HaveValue(Equal(constants.HSMLibVolumeName)))
	g.Expect(podSpec.Volumes[1].EmptyDir).ToNot(BeNil())
}
