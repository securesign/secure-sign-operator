package traits

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/utils/ptr"
)

func deploymentWithContainers(containerNames ...string) *appsv1ac.DeploymentApplyConfiguration {
	return appsv1ac.Deployment("test", "default").
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(1).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithSpec(podSpecWithContainers(containerNames...))))
}

func TestPodRequirements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		req    rhtasv1.PodRequirements
		verify func(Gomega, *appsv1ac.DeploymentApplyConfiguration)
	}{
		{
			name: "empty requirements preserve defaults",
			verify: func(g Gomega, dp *appsv1ac.DeploymentApplyConfiguration) {
				g.Expect(dp.Spec.Replicas).To(HaveValue(Equal(int32(1))))
				g.Expect(dp.Spec.Template.Spec.Affinity).To(BeNil())
				g.Expect(dp.Spec.Template.Spec.Tolerations).To(BeEmpty())
			},
		},
		{
			name: "replicas override",
			req:  rhtasv1.PodRequirements{Replicas: ptr.To(int32(3))},
			verify: func(g Gomega, dp *appsv1ac.DeploymentApplyConfiguration) {
				g.Expect(dp.Spec.Replicas).To(HaveValue(Equal(int32(3))))
			},
		},
		{
			name: "resources apply only to matching containers",
			req: rhtasv1.PodRequirements{Resources: &core.ResourceRequirements{
				Limits: core.ResourceList{core.ResourceCPU: resource.MustParse("200m")},
			}},
			verify: func(g Gomega, dp *appsv1ac.DeploymentApplyConfiguration) {
				containers := dp.Spec.Template.Spec.Containers
				g.Expect(containers[0].Resources).ToNot(BeNil())
				g.Expect((*containers[0].Resources.Limits)[core.ResourceCPU]).To(Equal(resource.MustParse("200m")))
				g.Expect(containers[1].Resources).To(BeNil())
			},
		},
		{
			name: "affinity",
			req: rhtasv1.PodRequirements{Affinity: &core.Affinity{
				NodeAffinity: &core.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
						NodeSelectorTerms: []core.NodeSelectorTerm{{
							MatchExpressions: []core.NodeSelectorRequirement{{
								Key: "zone", Operator: core.NodeSelectorOpIn, Values: []string{"east"},
							}},
						}},
					},
				},
			}},
			verify: func(g Gomega, dp *appsv1ac.DeploymentApplyConfiguration) {
				g.Expect(dp.Spec.Template.Spec.Affinity.NodeAffinity).ToNot(BeNil())
			},
		},
		{
			name: "tolerations",
			req: rhtasv1.PodRequirements{Tolerations: []core.Toleration{
				{Key: "key1", Operator: core.TolerationOpExists},
				{Key: "key2", Operator: core.TolerationOpEqual, Value: "value"},
			}},
			verify: func(g Gomega, dp *appsv1ac.DeploymentApplyConfiguration) {
				g.Expect(dp.Spec.Template.Spec.Tolerations).To(HaveLen(2))
				g.Expect(dp.Spec.Template.Spec.Tolerations[0].Key).To(HaveValue(Equal("key1")))
				g.Expect(dp.Spec.Template.Spec.Tolerations[1].Key).To(HaveValue(Equal("key2")))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			dp := deploymentWithContainers("server", "sidecar")
			g.Expect(PodRequirements(tt.req, "server")(dp)).To(Succeed())
			tt.verify(g, dp)
		})
	}
}

func TestWithPodSpecTraits(t *testing.T) {
	t.Parallel()
	t.Run("applies traits in order", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		dp := deploymentWithContainers("server")
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

		g.Expect(WithPodSpecTraits(first, second)(dp)).To(Succeed())
		g.Expect(calls).To(Equal([]string{"first", "second"}))
		g.Expect(dp.Spec.Template.Spec.ServiceAccountName).To(HaveValue(Equal("second")))
	})

	t.Run("stops after an error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		dp := deploymentWithContainers("server")
		expected := errors.New("trait failed")
		called := false

		failure := func(*corev1ac.PodSpecApplyConfiguration) error { return expected }
		after := func(*corev1ac.PodSpecApplyConfiguration) error {
			called = true
			return nil
		}

		g.Expect(WithPodSpecTraits(failure, after)(dp)).To(MatchError(expected))
		g.Expect(called).To(BeFalse())
	})
}
