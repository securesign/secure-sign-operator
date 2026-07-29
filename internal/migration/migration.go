// Package migration provides helpers for preserving v1alpha1-only fields across
// API version conversions using annotations under the migration.rhtas.redhat.com domain.
//
// When a field exists in v1alpha1 but not in v1 (the hub), the conversion webhook
// serializes it into a migration annotation on the v1 object. A controller action
// then reads the annotation and creates the appropriate replacement resource.
//
// Typical lifecycle of a migration annotation:
//
//  1. ConvertTo (spoke→hub):  migration.Set(hub, key, spokeField)
//  2. ConvertFrom (hub→spoke): migration.Pop(hub, key, &spokeField)
//     — removes it before MarshalData so it doesn't leak into conversion-data
//  3. Propagation:            ensure.Annotations or migration.Propagate copies
//     the annotation from a parent CR to the child CR that will act on it
//  4. Controller action:      migration.Has → migration.Read → create resource → migration.Remove
//
// Annotation key convention:
//
//	migration.rhtas.redhat.com/{apiVersion}.{fieldName}
//
// Use [Key] to build keys that follow this convention.
package migration

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const Prefix = "migration.rhtas.redhat.com/"

// Key builds a migration annotation key following the convention:
//
//	migration.rhtas.redhat.com/{apiVersion}.{field}
func Key(apiVersion, field string) string {
	return Prefix + apiVersion + "." + field
}

// Set marshals value as JSON and stores it under key in obj's annotations.
// Use in ConvertTo to preserve a spoke-only field on the hub object.
func Set(obj metav1.Object, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	a := obj.GetAnnotations()
	if a == nil {
		a = make(map[string]string)
	}
	a[key] = string(data)
	obj.SetAnnotations(a)
	return nil
}

// Read unmarshals the annotation value into target without modifying obj.
// Returns false if the annotation is absent or empty.
// Use in controller actions where removal is a separate step after successful processing.
func Read(obj metav1.Object, key string, target any) (bool, error) {
	a := obj.GetAnnotations()
	data, ok := a[key]
	if !ok || data == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return false, err
	}
	return true, nil
}

// Pop reads and removes the annotation in one step.
// Returns false if the annotation is absent or empty.
// Use in ConvertFrom to consume the annotation before MarshalData runs,
// preventing it from leaking into the conversion-data annotation.
func Pop(obj metav1.Object, key string, target any) (bool, error) {
	ok, err := Read(obj, key, target)
	if !ok || err != nil {
		return ok, err
	}
	Remove(obj, key)
	return true, nil
}

// Has reports whether the migration annotation exists on obj.
// Use in controller action CanHandle methods.
func Has(obj metav1.Object, key string) bool {
	a := obj.GetAnnotations()
	_, ok := a[key]
	return ok
}

// Remove deletes the migration annotation from obj.
// Use in controller actions after the migration has been successfully processed.
func Remove(obj metav1.Object, key string) {
	a := obj.GetAnnotations()
	delete(a, key)
	obj.SetAnnotations(a)
}

// StripAll removes all migration annotations from obj.
// Use in roundtrip and conversion tests where migration data must not
// survive the spoke→hub→spoke cycle.
func StripAll(obj metav1.Object) {
	a := obj.GetAnnotations()
	for k := range a {
		if strings.HasPrefix(k, Prefix) {
			delete(a, k)
		}
	}
	if len(a) == 0 {
		a = nil
	}
	obj.SetAnnotations(a)
}

// Propagate returns an ensure-style function that copies a migration annotation
// from srcAnnotations to the target object. Use in ensure_* actions to forward
// a migration annotation from a parent CR (e.g. Securesign) to the child CR
// (e.g. Rekor) whose controller will act on it.
func Propagate[T client.Object](key string, srcAnnotations map[string]string) func(T) error {
	return func(obj T) error {
		data, ok := srcAnnotations[key]
		if !ok || data == "" {
			return nil
		}
		a := obj.GetAnnotations()
		if a == nil {
			a = make(map[string]string)
		}
		a[key] = data
		obj.SetAnnotations(a)
		return nil
	}
}
