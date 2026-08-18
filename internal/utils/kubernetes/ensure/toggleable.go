package ensure

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/securesign/operator/internal/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// UserSpecifiedToggle automatically removes items when the user deletes them from the spec.
//
// Multiple calls per reconcile share a single annotation (LastUserSpecApplied).
// Each call's t.Managed declares which fields it owns. The implementation uses a
// reflection-based three-way merge (lastApplied, managed, live) to project only those
// owned fields from the annotation, diff against the current state, and remove stale items.
//
// Because multiple UserSpecifiedToggle calls per reconcile accumulate their contributions
// into this single annotation via object-merge/array-replace semantics, modular functions
// can manage disjoint top-level fields independently (e.g., Auth manages containers while
// PodExtensions manages volumes on the same PodSpec). Note: multiple toggles managing
// different sub-fields of the same array element (e.g., env and volumeMounts on the same
// container) will cause the later toggle to overwrite the earlier toggle's annotation data
// due to wholesale array replacement. Current usage (one toggle per resource type) avoids this.
//
// Alternatives Considered:
//
// Strategic merge patch was rejected: it cannot express "remove items absent from
// managed" without delete directives, and k8s.io/apimachinery/pkg/util/strategicpatch
// is deprecated.
//
// Server-Side Apply (SSA) was rejected: although it natively handles item-level
// ownership for arrays, our codebase relies on the CreateOrUpdate (PUT) lifecycle.
// Mixing SSA (Apply) with CreateOrUpdate (Update) on the same resource causes
// infinite ownership conflicts. Therefore, adopting SSA requires an all-or-nothing
// architectural shift to partial patch objects, which is not currently feasible.
//
// The reflection approach provides the required item-level merging and pruning
// in-memory, allowing us to safely manage these arrays while remaining fully
// compatible with our existing CreateOrUpdate patterns.
func UserSpecifiedToggle[T client.Object](t Toggleable[T]) func(T) error {
	return func(live T) error {
		stored := live.GetAnnotations()[annotations.LastUserSpecApplied]

		// Extract lastApplied for only the fields this call manages
		var lastAppliedTyped T
		if stored != "" {
			// Unmarshal full stored annotation
			fullLastApplied := reflect.New(reflect.TypeOf(live).Elem()).Interface().(T)
			if err := json.Unmarshal([]byte(stored), fullLastApplied); err != nil {
				// Invalid JSON - treat as empty to allow repair
				stored = ""
			} else {
				// Project down to only fields present in managed
				lastAppliedTyped = reflect.New(reflect.TypeOf(live).Elem()).Interface().(T)
				projectFields(lastAppliedTyped, fullLastApplied, t.Managed)
			}
		}

		// Remove stale items using reflection-based diff
		removeStale(live, lastAppliedTyped, t.Managed)

		// Run ensure to add/update managed items
		if err := t.Ensure(live); err != nil {
			return err
		}

		// Extract the current state of fields this call manages from live object
		currentManaged := reflect.New(reflect.TypeOf(live).Elem()).Interface().(T)
		projectManagedFields(currentManaged, live, t.Managed)

		// Merge into stored annotation, replacing (not upserting) this call's fields
		merged, err := mergeReplacingFields(stored, currentManaged)
		if err != nil {
			return err
		}

		return Annotations[T]([]string{annotations.LastUserSpecApplied},
			map[string]string{annotations.LastUserSpecApplied: merged},
		)(live)
	}
}

// projectFields copies fields from src to dst, but only for fields that are
// non-zero in mask. This extracts the subset of src that corresponds to the
// fields managed by the current call, as indicated by mask.
func projectFields(dst, src, mask any) {
	projectFieldsWithSliceFunc(dst, src, mask, projectSlice)
}

// projectManagedFields copies fields from src to dst, but only items whose names
// appear in mask. Used to extract current managed state after Ensure.
func projectManagedFields(dst, src, mask any) {
	projectFieldsWithSliceFunc(dst, src, mask, projectManagedSlice)
}

// projectFieldsWithSliceFunc is the implementation for both project variants.
func projectFieldsWithSliceFunc(dst, src, mask any, sliceFunc func(reflect.Value, reflect.Value, reflect.Value)) {
	dv := deref(reflect.ValueOf(dst))
	sv := deref(reflect.ValueOf(src))
	mv := deref(reflect.ValueOf(mask))

	if !dv.IsValid() || !sv.IsValid() || !mv.IsValid() {
		return
	}
	if dv.Kind() != reflect.Struct || sv.Kind() != reflect.Struct || mv.Kind() != reflect.Struct {
		return
	}
	if dv.Type() != sv.Type() || dv.Type() != mv.Type() {
		return
	}

	for i := range mv.NumField() {
		df := dv.Field(i)
		sf := sv.Field(i)
		mf := mv.Field(i)

		if !df.CanSet() {
			continue
		}

		switch mf.Kind() {
		case reflect.Slice:
			// Empty slices are meaningful - they indicate "manage this field with empty list"
			// so we need to project even when mf.Len() == 0
			if !mf.IsNil() {
				sliceFunc(df, sf, mf)
			}

		case reflect.Struct:
			if mf.IsZero() {
				continue
			}
			if !df.CanAddr() || !sf.CanAddr() {
				continue
			}
			projectFieldsWithSliceFunc(df.Addr().Interface(), sf.Addr().Interface(), mf.Addr().Interface(), sliceFunc)

		case reflect.Pointer:
			if mf.IsNil() {
				continue
			}
			if sf.IsNil() {
				continue
			}
			// Allocate destination pointer if nil
			if df.IsNil() {
				df.Set(reflect.New(df.Type().Elem()))
			}
			projectFieldsWithSliceFunc(df.Interface(), sf.Interface(), mf.Interface(), sliceFunc)

		default:
			// For scalar fields, only copy if mask has a non-zero value
			if !mf.IsZero() && sf.IsValid() {
				df.Set(sf)
			}
		}
	}
}

// projectSlice copies all items from src to dst if mask is non-nil.
// The mask indicates this call manages this field; we copy all historical values
// so removeStale can diff them against current managed values.
// Empty mask slice means "manage with empty list" - still copy src for cleanup.
func projectSlice(dst, src, mask reflect.Value) {
	if !dst.CanSet() || src.Type() != dst.Type() || mask.Type() != dst.Type() {
		return
	}
	if mask.IsNil() {
		return
	}

	// If this call manages this field (mask non-nil, even if empty), copy ALL items
	// from src so removeStale can see what was previously applied and remove stale items
	dst.Set(src)
}

// projectManagedSlice copies only items from src whose names appear in mask,
// recursing into nested fields to project those as well.
// Used to extract the current managed state from live after Ensure runs.
// Empty mask means "manage with empty list" - set dst to empty slice.
func projectManagedSlice(dst, src, mask reflect.Value) {
	if !dst.CanSet() || src.Type() != dst.Type() || mask.Type() != dst.Type() {
		return
	}
	if mask.IsNil() {
		return
	}
	if mask.Len() == 0 {
		// Empty managed list - set dst to empty slice
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
		return
	}

	elemType := mask.Type().Elem()
	nameField := nameFieldIndex(elemType)

	// For unnamed slices, copy all
	if nameField < 0 {
		dst.Set(src)
		return
	}

	// Build index of managed items by name
	managedByName := make(map[string]reflect.Value, mask.Len())
	for i := range mask.Len() {
		item := mask.Index(i)
		name := item.Field(nameField).String()
		managedByName[name] = item
	}

	// Build index of source items by name
	srcByName := make(map[string]reflect.Value, src.Len())
	for i := range src.Len() {
		item := src.Index(i)
		name := item.Field(nameField).String()
		srcByName[name] = item
	}

	// Copy only items whose names are in managed, recursing if needed
	// Sort names for deterministic ordering
	names := make([]string, 0, len(managedByName))
	for name := range managedByName {
		names = append(names, name)
	}
	slices.Sort(names)

	result := reflect.MakeSlice(dst.Type(), 0, len(managedByName))
	for _, name := range names {
		maskItem := managedByName[name]
		srcItem, exists := srcByName[name]
		if !exists {
			continue
		}
		// Create a new item and recursively project its fields
		newItem := reflect.New(elemType).Elem()
		projectFieldsWithSliceFunc(newItem.Addr().Interface(), srcItem.Addr().Interface(), maskItem.Addr().Interface(), projectManagedSlice)
		result = reflect.Append(result, newItem)
	}
	dst.Set(result)
}

// mergeReplacingFields merges currentManaged into stored, REPLACING (not upserting)
// the fields present in currentManaged. Returns the merged JSON string with empty fields removed.
func mergeReplacingFields(stored string, currentManaged any) (string, error) {
	currentJSON, err := json.Marshal(currentManaged)
	if err != nil {
		return "", err
	}

	var currentObj any
	if err := json.Unmarshal(currentJSON, &currentObj); err != nil {
		return "", err
	}

	// Inject empty arrays for non-nil empty slices (omitted by json:",omitempty")
	injectEmptySlices(currentObj, currentManaged)

	if stored == "" {
		cleaned := removeEmptyFields(currentObj)
		result, err := json.Marshal(cleaned)
		return string(result), err
	}

	// Parse stored annotation
	var storedObj any
	if err := json.Unmarshal([]byte(stored), &storedObj); err != nil {
		return "", err
	}

	// Replace fields this call manages, using uncleaned currentObj as mask
	merged := replaceJSONFields(storedObj, currentObj, currentObj)
	// Remove empty fields from final result
	cleaned := removeEmptyFields(merged)
	result, err := json.Marshal(cleaned)
	return string(result), err
}

// injectEmptySlices walks the reflect value and injects empty arrays into the JSON
// object for slice fields that are non-nil but empty (omitted by json:",omitempty").
func injectEmptySlices(jsonObj any, reflectVal any) {
	obj, ok := jsonObj.(map[string]any)
	if !ok {
		return
	}

	rv := deref(reflect.ValueOf(reflectVal))
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return
	}

	for i := range rv.NumField() {
		field := rv.Field(i)
		fieldType := rv.Type().Field(i)

		// Skip unexported fields
		if !fieldType.IsExported() {
			continue
		}

		jsonName := fieldType.Name
		if tag, ok := fieldType.Tag.Lookup("json"); ok {
			parts := strings.Split(tag, ",")
			if parts[0] != "" && parts[0] != "-" {
				jsonName = parts[0]
			}
		}

		if field.Kind() == reflect.Slice && !field.IsNil() && field.Len() == 0 {
			// Non-nil empty slice - inject empty array if omitted
			if _, exists := obj[jsonName]; !exists {
				obj[jsonName] = []any{}
			}
		} else if field.Kind() == reflect.Struct || (field.Kind() == reflect.Ptr && !field.IsNil() && field.Elem().Kind() == reflect.Struct) {
			// Recurse into nested structs
			if nestedObj, ok := obj[jsonName].(map[string]any); ok {
				injectEmptySlices(nestedObj, field.Interface())
			}
		}
	}
}

// removeEmptyFields recursively removes empty objects, empty arrays, and null values
// from a JSON object tree. Used to clean up annotations.
func removeEmptyFields(v any) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any)
		for k, v := range val {
			cleaned := removeEmptyFields(v)
			if !isEmptyValue(cleaned) {
				result[k] = cleaned
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	case []any:
		if len(val) == 0 {
			return nil
		}
		result := make([]any, 0, len(val))
		for _, item := range val {
			cleaned := removeEmptyFields(item)
			if !isEmptyValue(cleaned) {
				result = append(result, cleaned)
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	default:
		return v
	}
}

// isEmptyValue returns true if v is nil, an empty map, or an empty array.
func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case map[string]any:
		return len(val) == 0
	case []any:
		return len(val) == 0
	}
	return false
}

// replaceJSONFields merges src into dst, but REPLACES fields present in mask
// instead of merging them. Used for annotation updates where we want to replace
// entire arrays, not upsert items.
func replaceJSONFields(dst, src, mask any) any {
	if mask == nil {
		return dst
	}

	dstObj, dstIsObj := dst.(map[string]any)
	srcObj, srcIsObj := src.(map[string]any)
	maskObj, maskIsObj := mask.(map[string]any)

	if dstIsObj && srcIsObj && maskIsObj {
		merged := maps.Clone(dstObj)
		for k, maskVal := range maskObj {
			if maskVal == nil {
				continue
			}
			srcVal, hasSrc := srcObj[k]
			if !hasSrc {
				continue
			}
			// If mask has this field, check if we should recurse or replace
			if maskValObj, ok := maskVal.(map[string]any); ok {
				// Recurse into nested objects
				merged[k] = replaceJSONFields(merged[k], srcVal, maskValObj)
			} else {
				// Replace wholesale (for arrays and scalars)
				merged[k] = srcVal
			}
		}
		return merged
	}

	// For non-objects, replace if mask is non-nil
	return src
}

// removeStale removes items from live that were declared in lastApplied but are no
// longer declared in managed. Named items (Volume, Container, EnvVar, ...) still present
// in managed are kept and recursed into, so stale nested items are cleaned without
// removing the parent; named items absent from managed are removed outright, regardless
// of whether they're a leaf type or hold further nested named slices.
func removeStale(live, lastApplied, managed any) {
	lv := deref(reflect.ValueOf(live))
	av := deref(reflect.ValueOf(lastApplied))
	mv := deref(reflect.ValueOf(managed))
	if !lv.IsValid() || !av.IsValid() {
		return
	}
	if lv.Kind() != reflect.Struct || av.Kind() != reflect.Struct || lv.Type() != av.Type() {
		return
	}
	if !mv.IsValid() || mv.Kind() != reflect.Struct || mv.Type() != lv.Type() {
		mv = reflect.Zero(lv.Type())
	}

	for i := range av.NumField() {
		lf := lv.Field(i)
		af := av.Field(i)
		mf := mv.Field(i)

		switch af.Kind() {
		case reflect.Slice:
			if af.Len() == 0 || !lf.CanSet() {
				continue
			}
			removeStaleSlice(lf, af, mf)

		case reflect.Struct:
			if !lf.CanAddr() {
				continue
			}
			removeStale(lf.Addr().Interface(), af.Addr().Interface(), mf.Addr().Interface())

		case reflect.Pointer:
			if af.IsNil() || lf.IsNil() {
				continue
			}
			var managedArg any
			if mf.Kind() == reflect.Pointer && !mf.IsNil() {
				managedArg = mf.Interface()
			}
			removeStale(lf.Interface(), af.Interface(), managedArg)
		}
	}
}

// removeStaleSlice mutates live in place, removing items declared in lastApplied but
// dropped from managed. See removeStale for the recurse-vs-remove distinction.
func removeStaleSlice(live, lastApplied, managed reflect.Value) {
	elemType := lastApplied.Type().Elem()
	nameField := nameFieldIndex(elemType)

	if nameField < 0 {
		removeByValue(live, diffByValue(lastApplied, managed))
		return
	}

	lastAppliedByName := make(map[string]reflect.Value, lastApplied.Len())
	for i := range lastApplied.Len() {
		item := lastApplied.Index(i)
		lastAppliedByName[item.Field(nameField).String()] = item
	}
	managedByName := make(map[string]reflect.Value, managed.Len())
	for i := range managed.Len() {
		item := managed.Index(i)
		managedByName[item.Field(nameField).String()] = item
	}

	filtered := reflect.MakeSlice(live.Type(), 0, live.Len())
	for i := range live.Len() {
		item := live.Index(i)
		name := item.Field(nameField).String()

		lastAppliedItem, wasApplied := lastAppliedByName[name]
		if !wasApplied {
			filtered = reflect.Append(filtered, item)
			continue
		}

		managedItem, stillManaged := managedByName[name]
		if !stillManaged {
			continue
		}

		removeStale(item.Addr().Interface(), lastAppliedItem.Addr().Interface(), managedItem.Addr().Interface())
		filtered = reflect.Append(filtered, item)
	}
	live.Set(filtered)
}

// diffByValue returns the items in lastApplied that have no DeepEqual match in managed.
// Used for slices of non-named types (e.g. []string for args).
func diffByValue(lastApplied, managed reflect.Value) reflect.Value {
	result := reflect.MakeSlice(lastApplied.Type(), 0, lastApplied.Len())
	for i := range lastApplied.Len() {
		found := false
		for j := range managed.Len() {
			if reflect.DeepEqual(lastApplied.Index(i).Interface(), managed.Index(j).Interface()) {
				found = true
				break
			}
		}
		if !found {
			result = reflect.Append(result, lastApplied.Index(i))
		}
	}
	return result
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
