package traits

import (
	"fmt"

	apps "k8s.io/api/apps/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

// StatefulSetSelector sets the immutable spec.selector and the pod template
// labels to the same set of labels.
func StatefulSetSelector(labels map[string]string) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		spec := statefulSetSpec(ss)
		spec.WithSelector(metav1ac.LabelSelector().WithMatchLabels(labels))
		if spec.Template == nil {
			spec.WithTemplate(corev1ac.PodTemplateSpec())
		}
		spec.Template.WithLabels(labels)
		return nil
	}
}

// ServiceName sets the immutable spec.serviceName governing the StatefulSet's
// network identity.
func ServiceName(name string) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		statefulSetSpec(ss).WithServiceName(name)
		return nil
	}
}

// UpdateStrategy sets the StatefulSet update strategy. Declare the complete
// strategy: the API server only defaults rollingUpdate.partition when the type
// is unset, so a type-only declaration prunes a previously defaulted partition.
func UpdateStrategy(strategy *appsv1ac.StatefulSetUpdateStrategyApplyConfiguration) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		statefulSetSpec(ss).WithUpdateStrategy(strategy)
		return nil
	}
}

// PodManagementPolicy sets the immutable spec.podManagementPolicy.
func PodManagementPolicy(policy apps.PodManagementPolicyType) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		statefulSetSpec(ss).WithPodManagementPolicy(policy)
		return nil
	}
}

// VolumeClaimTemplates sets spec.volumeClaimTemplates. The list is atomic and
// immutable, so callers must declare exactly the templates the StatefulSet was
// created with. Build templates with a bare PersistentVolumeClaimApplyConfiguration
// (not corev1ac.PersistentVolumeClaim) so no kind/apiVersion/namespace is sent.
func VolumeClaimTemplates(templates ...*corev1ac.PersistentVolumeClaimApplyConfiguration) Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		statefulSetSpec(ss).WithVolumeClaimTemplates(templates...)
		return nil
	}
}

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

func statefulSetSpec(ss *appsv1ac.StatefulSetApplyConfiguration) *appsv1ac.StatefulSetSpecApplyConfiguration {
	if ss.Spec == nil {
		ss.WithSpec(appsv1ac.StatefulSetSpec())
	}
	return ss.Spec
}
