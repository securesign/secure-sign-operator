package traits

import (
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// WithJobPodSpecTraits applies pod traits to a Job's pod template.
func WithJobPodSpecTraits(traits ...Trait[*corev1ac.PodSpecApplyConfiguration]) Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
		if job.Spec == nil {
			job.WithSpec(batchv1ac.JobSpec())
		}
		if job.Spec.Template == nil {
			job.Spec.WithTemplate(corev1ac.PodTemplateSpec())
		}
		if job.Spec.Template.Spec == nil {
			job.Spec.Template.WithSpec(corev1ac.PodSpec())
		}

		_, err := Apply(job.Spec.Template.Spec, traits...)
		return err
	}
}

func WithCronJobPodSpecTraits(traits ...Trait[*corev1ac.PodSpecApplyConfiguration]) Trait[*batchv1ac.CronJobApplyConfiguration] {
	return func(cronJob *batchv1ac.CronJobApplyConfiguration) error {
		if cronJob.Spec == nil {
			cronJob.WithSpec(batchv1ac.CronJobSpec())
		}
		if cronJob.Spec.JobTemplate == nil {
			cronJob.Spec.WithJobTemplate(batchv1ac.JobTemplateSpec())
		}
		if cronJob.Spec.JobTemplate.Spec == nil {
			cronJob.Spec.JobTemplate.WithSpec(batchv1ac.JobSpec())
		}
		_, err := Apply(cronJob.Spec.JobTemplate.Spec.Template.Spec, traits...)
		return err
	}
}
