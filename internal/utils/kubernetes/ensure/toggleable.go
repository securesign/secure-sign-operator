package ensure

import (
	"reflect"
)

// Toggleable wraps an ensure function with a declaration of what it manages.
// When the condition is true, Enable runs. When false, OptionalToggle
// auto-generates cleanup by removing items declared in Managed from the live object.
//
// Managed is a partial object of the same type T — only populate the fields
// (slices of named structs) that this ensure function manages. The reflect-based
// cleanup walks Managed and removes matching items by Name from the live object.
type Toggleable[T any] struct {
	Ensure  func(T) error
	Managed T
}

// OptionalToggle applies the Toggleable's Enable function when condition is true.
// When false, it removes all items declared in Managed from the live object using
// reflect-based name matching.
func OptionalToggle[T any](condition bool, t Toggleable[T]) func(T) error {
	if condition {
		return t.Ensure
	}
	return func(obj T) error {
		removeManaged(obj, t.Managed)
		return nil
	}
}

// EnsureWithCleanup applies t.Ensure to the target and then removes items
// declared in t.Managed. Unlike OptionalToggle, it always runs both steps.
// Use it when Managed represents stale items to remove after upserting desired state.
func EnsureWithCleanup[T any](target T, t Toggleable[T]) error {
	if err := t.Ensure(target); err != nil {
		return err
	}
	removeManaged(target, t.Managed)
	return nil
}

// removeManaged removes items declared in managed from the live object.
// It uses reflect to match items by Name across all nested slices (Volumes,
// Containers, VolumeMounts, Env, Ports, etc.) regardless of resource type.
func removeManaged(live, managed any) {
	lv := deref(reflect.ValueOf(live))
	mv := deref(reflect.ValueOf(managed))
	if !lv.IsValid() || !mv.IsValid() {
		return
	}
	if lv.Kind() != reflect.Struct || mv.Kind() != reflect.Struct || lv.Type() != mv.Type() {
		return
	}

	for i := range mv.NumField() {
		// both values are the same type T, so Field(i) refers to the same field.
		mf := mv.Field(i)
		lf := lv.Field(i)

		if !lf.CanSet() && lf.Kind() != reflect.Pointer && lf.Kind() != reflect.Struct {
			continue
		}

		switch mf.Kind() {
		case reflect.Slice:
			if mf.Len() == 0 {
				continue
			}
			processSlice(lf, mf)

		case reflect.Struct:
			if mf.IsZero() {
				continue
			}
			removeManaged(lf.Addr().Interface(), mf.Addr().Interface())

		case reflect.Pointer:
			if mf.IsNil() {
				continue
			}
			removeManaged(lf.Interface(), mf.Interface())
		}
	}
}

// processSlice handles a slice field from the managed object against the live object.
// It distinguishes two cases based on whether the slice elements have a Name field:
//
//  1. Named elements (Volume, Container, EnvVar, etc.): items in live whose Name
//     matches any managed item are either removed outright (leaf items like Volume,
//     EnvVar) or recursed into (container-like items that hold nested named slices).
//
//  2. Unnamed elements: the managed slice items are removed from live by value equality.
func processSlice(live, managed reflect.Value) {
	if !live.CanSet() || live.Type() != managed.Type() {
		return
	}
	if managed.Len() == 0 {
		return
	}

	elemType := managed.Type().Elem()
	nameField := nameFieldIndex(elemType)

	// if there is no Name field, remove by value
	if nameField < 0 {
		removeByValue(live, managed)
		return
	}

	managedNames := make(map[string]reflect.Value, managed.Len())
	for i := range managed.Len() {
		name := managed.Index(i).Field(nameField).String()
		managedNames[name] = managed.Index(i)
	}

	hasNestedSlices := hasNamedSliceFields(elemType)

	if hasNestedSlices {
		// managed is not a leaf type - we need to recursively remove items from nested
		for i := range live.Len() {
			name := live.Index(i).Field(nameField).String()
			if managedItem, ok := managedNames[name]; ok {
				removeManaged(live.Index(i).Addr().Interface(), managedItem.Addr().Interface())
			}
		}
	} else {
		// managed is a leaf type - remove right away
		filtered := reflect.MakeSlice(live.Type(), 0, live.Len())
		for i := range live.Len() {
			name := live.Index(i).Field(nameField).String()
			if _, shouldRemove := managedNames[name]; !shouldRemove {
				filtered = reflect.Append(filtered, live.Index(i))
			}
		}
		live.Set(filtered)
	}
}

// removeByValue removes items from live that are present in managed, compared by
// reflect.DeepEqual. Used for slices of non-named types (e.g., []string for args).
func removeByValue(live, managed reflect.Value) {
	if !live.CanSet() {
		return
	}
	filtered := reflect.MakeSlice(live.Type(), 0, live.Len())
	for i := range live.Len() {
		found := false
		for j := range managed.Len() {
			if reflect.DeepEqual(live.Index(i).Interface(), managed.Index(j).Interface()) {
				found = true
				break
			}
		}
		if !found {
			filtered = reflect.Append(filtered, live.Index(i))
		}
	}
	live.Set(filtered)
}

// nameFieldIndex returns the index of the "Name" field in a struct type and
// whether it exists. Most Kubernetes PodSpec list items (Volume, Container,
// EnvVar, VolumeMount, ContainerPort) have a Name field.
func nameFieldIndex(t reflect.Type) int {
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return -1
	}
	for i := range t.NumField() {
		if t.Field(i).Name == "Name" && t.Field(i).Type.Kind() == reflect.String {
			return i
		}
	}
	return -1
}

// hasNamedSliceFields reports whether a struct type contains any slice fields
// whose elements are structs with a Name field. This distinguishes "container-like"
// types (which hold nested named slices like VolumeMounts, Env, Ports) from
// "leaf" types (like Volume, EnvVar) that should be removed outright.
func hasNamedSliceFields(t reflect.Type) bool {
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Slice {
			elemType := f.Type.Elem()
			if elemType.Kind() == reflect.Struct {
				if nameFieldIndex(elemType) >= 0 {
					return true
				}
			}
		}
	}
	return false
}

// deref dereferences a reflect.Value through any number of pointer indirections.
func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// derefType dereferences a reflect.Type through any number of pointer indirections.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
