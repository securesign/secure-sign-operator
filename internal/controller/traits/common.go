package traits

import (
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type Labelable[T any] interface {
	WithLabels(map[string]string) T
}

type Annotatable[T any] interface {
	WithAnnotations(map[string]string) T
}

type OwnerReferenceable[T any] interface {
	WithOwnerReferences(values ...*metav1ac.OwnerReferenceApplyConfiguration) T
}

func Labels[T Labelable[T]](labels map[string]string) Trait[T] {
	return func(obj T) error {
		obj.WithLabels(labels)
		return nil
	}
}

func Annotations[T Annotatable[T]](annotations map[string]string) Trait[T] {
	return func(obj T) error {
		obj.WithAnnotations(annotations)
		return nil
	}
}

func ControllerReference[T OwnerReferenceable[T]](owner client.Object, cli client.Client) Trait[T] {
	return func(obj T) error {
		gvk, err := apiutil.GVKForObject(owner, cli.Scheme())
		if err != nil {
			return err
		}
		obj.WithOwnerReferences(metav1ac.OwnerReference().
			WithAPIVersion(gvk.GroupVersion().String()).
			WithKind(gvk.Kind).
			WithName(owner.GetName()).
			WithUID(owner.GetUID()).
			WithController(true).
			WithBlockOwnerDeletion(true))
		return nil
	}
}

func Optional[T any](condition bool, fn Trait[T]) Trait[T] {
	if condition {
		return fn
	}
	return func(obj T) error {
		return nil
	}
}
