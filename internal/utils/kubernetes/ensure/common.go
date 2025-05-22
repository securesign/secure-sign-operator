package ensure

import (
	"maps"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func managedDeleteFunction(managed []string) func(string, string) bool {
	return func(key, _ string) bool {
		return slices.Contains(managed, key)
	}
}

func Labels[T client.Object](managedLabels []string, labels map[string]string) func(T) error {
	return func(obj T) (e error) {
		if obj.GetLabels() == nil {
			obj.SetLabels(maps.Clone(labels))
			return
		}
		maps.DeleteFunc(obj.GetLabels(), managedDeleteFunction(managedLabels))
		maps.Copy(obj.GetLabels(), labels)
		return
	}
}

func Annotations[T client.Object](managedAnnotations []string, annotations map[string]string) func(T) error {
	return func(obj T) (e error) {
		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(maps.Clone(annotations))
			return
		}
		maps.DeleteFunc(obj.GetAnnotations(), managedDeleteFunction(managedAnnotations))
		maps.Copy(obj.GetAnnotations(), annotations)
		return
	}
}

func ControllerReference[T client.Object](owner client.Object, cli client.Client) func(controlled T) error {
	return func(controlled T) error {
		return controllerutil.SetControllerReference(owner, controlled, cli.Scheme())
	}
}

func Optional[T client.Object](condition bool, fn func(controlled T) error) func(controlled T) error {
	if condition {
		return fn
	} else {
		// return empty function
		return func(controlled T) error {
			return nil
		}
	}
}
