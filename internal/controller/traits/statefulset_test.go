package traits

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func statefulSetWithContainers(containerNames ...string) *appsv1ac.StatefulSetApplyConfiguration {
	return appsv1ac.StatefulSet("test", "default").
		WithSpec(appsv1ac.StatefulSetSpec().
			WithTemplate(corev1ac.PodTemplateSpec().
				WithSpec(podSpecWithContainers(containerNames...))))
}

func TestStatefulSetSelector(t *testing.T) {
	t.Parallel()
	labels := map[string]string{"app": "monitor"}

	t.Run("sets selector and template labels", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ss := statefulSetWithContainers("server")

		g.Expect(StatefulSetSelector(labels)(ss)).To(Succeed())
		g.Expect(ss.Spec.Selector.MatchLabels).To(Equal(labels))
		g.Expect(ss.Spec.Template.Labels).To(Equal(labels))
		g.Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(1), "existing pod spec is preserved")
	})

	t.Run("initializes empty spec", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ss := appsv1ac.StatefulSet("test", "default")

		g.Expect(StatefulSetSelector(labels)(ss)).To(Succeed())
		g.Expect(ss.Spec.Selector.MatchLabels).To(Equal(labels))
		g.Expect(ss.Spec.Template.Labels).To(Equal(labels))
	})
}

func TestStatefulSetFields(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ss := appsv1ac.StatefulSet("test", "default")

	_, err := Apply(ss,
		ServiceName("monitor"),
		UpdateStrategy(appsv1ac.StatefulSetUpdateStrategy().
			WithType(apps.RollingUpdateStatefulSetStrategyType).
			WithRollingUpdate(appsv1ac.RollingUpdateStatefulSetStrategy().WithPartition(0))),
		PodManagementPolicy(apps.OrderedReadyPodManagement),
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ss.Spec.ServiceName).To(HaveValue(Equal("monitor")))
	g.Expect(ss.Spec.UpdateStrategy.Type).To(HaveValue(Equal(apps.RollingUpdateStatefulSetStrategyType)))
	g.Expect(ss.Spec.UpdateStrategy.RollingUpdate.Partition).To(HaveValue(Equal(int32(0))))
	g.Expect(ss.Spec.PodManagementPolicy).To(HaveValue(Equal(apps.OrderedReadyPodManagement)))
}

func TestVolumeClaimTemplates(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ss := appsv1ac.StatefulSet("test", "default")

	pvc := (&corev1ac.PersistentVolumeClaimApplyConfiguration{}).
		WithName("storage").
		WithSpec(corev1ac.PersistentVolumeClaimSpec().
			WithAccessModes(core.ReadWriteOnce).
			WithResources(corev1ac.VolumeResourceRequirements().
				WithRequests(core.ResourceList{core.ResourceStorage: resource.MustParse("5Mi")})))

	g.Expect(VolumeClaimTemplates(pvc)(ss)).To(Succeed())
	g.Expect(ss.Spec.VolumeClaimTemplates).To(HaveLen(1))
	template := ss.Spec.VolumeClaimTemplates[0]
	g.Expect(template.Name).To(HaveValue(Equal("storage")))
	g.Expect(template.Kind).To(BeNil(), "kind must not be sent in an immutable template")
	g.Expect(template.APIVersion).To(BeNil(), "apiVersion must not be sent in an immutable template")
	g.Expect(template.Namespace).To(BeNil(), "namespace must not be sent in an immutable template")
	g.Expect(template.Spec.AccessModes).To(ConsistOf(core.ReadWriteOnce))
}

func TestWithStatefulSetPodSpecTraits(t *testing.T) {
	t.Parallel()

	t.Run("applies traits in order", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ss := statefulSetWithContainers("server")
		var calls []string

		first := func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
			calls = append(calls, "first")
			podSpec.WithServiceAccountName("first")
			return nil
		}
		second := func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
			calls = append(calls, "second")
			podSpec.WithServiceAccountName("second")
			return nil
		}

		g.Expect(WithStatefulSetPodSpecTraits(first, second)(ss)).To(Succeed())
		g.Expect(calls).To(Equal([]string{"first", "second"}))
		g.Expect(ss.Spec.Template.Spec.ServiceAccountName).To(HaveValue(Equal("second")))
	})

	t.Run("stops after an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ss := statefulSetWithContainers("server")
		expected := errors.New("trait failed")
		called := false

		failure := func(*corev1ac.PodSpecApplyConfiguration) error { return expected }
		after := func(*corev1ac.PodSpecApplyConfiguration) error {
			called = true
			return nil
		}

		g.Expect(WithStatefulSetPodSpecTraits(failure, after)(ss)).To(MatchError(expected))
		g.Expect(called).To(BeFalse())
	})

	t.Run("requires pod spec", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		ss := appsv1ac.StatefulSet("test", "default")

		g.Expect(WithStatefulSetPodSpecTraits()(ss)).To(MatchError(ContainSubstring("pod spec is not configured")))
	})
}
