package traits

import (
	"fmt"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	core "k8s.io/api/core/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// PodRequirements applies replicas, resources, affinity, and tolerations
// from a PodRequirements spec to the deployment.
func PodRequirements(req rhtasv1.PodRequirements, containerName ...string) Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		if dp.Spec == nil || dp.Spec.Template == nil || dp.Spec.Template.Spec == nil {
			return fmt.Errorf("deployment pod spec is not configured")
		}

		if req.Replicas != nil {
			dp.Spec.WithReplicas(*req.Replicas)
		} else {
			dp.Spec.WithReplicas(1)
		}

		podSpec := dp.Spec.Template.Spec
		if req.Resources != nil {
			res, err := ConvertViaJSON[core.ResourceRequirements, corev1ac.ResourceRequirementsApplyConfiguration](*req.Resources)
			if err != nil {
				return fmt.Errorf("convert resource requirements: %w", err)
			}
			for i := range podSpec.Containers {
				if podSpec.Containers[i].Name != nil && slices.Contains(containerName, *podSpec.Containers[i].Name) {
					podSpec.Containers[i].WithResources(res)
				}
			}
		}

		if req.Affinity != nil {
			affinity, err := ConvertViaJSON[core.Affinity, corev1ac.AffinityApplyConfiguration](*req.Affinity)
			if err != nil {
				return fmt.Errorf("convert affinity: %w", err)
			}
			podSpec.WithAffinity(affinity)
		}

		if len(req.Tolerations) > 0 {
			for _, t := range req.Tolerations {
				tAC, err := ConvertViaJSON[core.Toleration, corev1ac.TolerationApplyConfiguration](t)
				if err != nil {
					return fmt.Errorf("convert toleration: %w", err)
				}
				podSpec.WithTolerations(tAC)
			}
		}

		return nil
	}
}

func WithPodSpecTraits(traits ...Trait[*corev1ac.PodSpecApplyConfiguration]) Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		if dp.Spec == nil || dp.Spec.Template == nil || dp.Spec.Template.Spec == nil {
			return fmt.Errorf("deployment pod spec is not configured")
		}
		_, err := Apply(dp.Spec.Template.Spec, traits...)
		return err
	}
}
