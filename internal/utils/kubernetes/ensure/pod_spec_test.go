package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/config"
	core "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const testContainerName = "app"
const initContainerName = "init"

// TestPodSecurityContext_Kubernetes verifies runAsUser / fsGroup are set when
// not on OpenShift and cleared when on OpenShift.
func TestPodSecurityContext_PlatformMode(t *testing.T) {
	origOpenshift := config.Openshift
	t.Cleanup(func() { config.Openshift = origOpenshift })

	t.Run("kubernetes mode sets fsGroup and runAsUser when nil", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		spec := core.PodSpec{
			Containers:     []core.Container{{Name: testContainerName}},
			InitContainers: []core.Container{{Name: initContainerName}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(1001))))
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))))
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))))
	})

	t.Run("kubernetes mode does not overwrite existing fsGroup or runAsUser", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		spec := core.PodSpec{
			SecurityContext: &core.PodSecurityContext{FSGroup: ptr.To(int64(2000))},
			Containers: []core.Container{{
				Name:            testContainerName,
				SecurityContext: &core.SecurityContext{RunAsUser: ptr.To(int64(2000))},
			}},
			InitContainers: []core.Container{{
				Name:            initContainerName,
				SecurityContext: &core.SecurityContext{RunAsUser: ptr.To(int64(2000))},
			}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(2000))))
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(2000))))
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(2000))))
	})

	t.Run("openshift mode does not set fsGroup or runAsUser", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		spec := core.PodSpec{
			Containers:     []core.Container{{Name: testContainerName}},
			InitContainers: []core.Container{{Name: initContainerName}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(BeNil())
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(BeNil())
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(BeNil())
	})

	t.Run("openshift mode clears stale fsGroup and runAsUser", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		spec := core.PodSpec{
			SecurityContext: &core.PodSecurityContext{FSGroup: ptr.To(int64(1001))},
			Containers: []core.Container{{
				Name: testContainerName,
				SecurityContext: &core.SecurityContext{
					RunAsUser: ptr.To(int64(1001)),
				},
			}},
			InitContainers: []core.Container{{
				Name: initContainerName,
				SecurityContext: &core.SecurityContext{
					RunAsUser: ptr.To(int64(1001)),
				},
			}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(BeNil())
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(BeNil())
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(BeNil())
	})
}
