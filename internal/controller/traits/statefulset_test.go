package traits

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func statefulSetWithContainers(containerNames ...string) *appsv1ac.StatefulSetApplyConfiguration {
	return appsv1ac.StatefulSet("test", "default").
		WithSpec(appsv1ac.StatefulSetSpec().
			WithTemplate(corev1ac.PodTemplateSpec().
				WithSpec(podSpecWithContainers(containerNames...))))
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
