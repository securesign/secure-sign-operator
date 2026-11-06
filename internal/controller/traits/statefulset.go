package traits

import (
	"fmt"

	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// WithStatefulSetPodSpecTraits applies pod spec traits to the StatefulSet pod template.
func WithStatefulSetPodSpecTraits(traits ...Trait[*corev1ac.PodSpecApplyConfiguration]) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		if ss.Spec == nil || ss.Spec.Template == nil || ss.Spec.Template.Spec == nil {
			return fmt.Errorf("statefulset pod spec is not configured")
		}
		_, err := Apply(ss.Spec.Template.Spec, traits...)
		return err
	}
}
