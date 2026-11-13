package traits

import (
	"fmt"

	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// WithJobPodSpecTraits applies pod traits to a Job's pod template.
func WithJobPodSpecTraits(traits ...Trait[*corev1ac.PodSpecApplyConfiguration]) Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
		if job.Spec == nil || job.Spec.Template == nil || job.Spec.Template.Spec == nil {
			return fmt.Errorf("job pod spec is not configured")
		}
		_, err := Apply(job.Spec.Template.Spec, traits...)
		return err
	}
}
