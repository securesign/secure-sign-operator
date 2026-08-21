package ensure

import (
	"encoding/json"
	"sort"

	v1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// podExtensionsConcern is this file's key within the namespaced
// LastUserSpecApplied annotation. See ReadNamespacedState.
const podExtensionsConcern = "podExtensions"

// UserSpecState tracks the names of user-specified volumes and volume mounts
// that were applied during the last reconcile. Stored under the
// "podExtensions" key of the LastUserSpecApplied annotation.
type UserSpecState struct {
	Volumes      []string `json:"volumes,omitempty"`
	VolumeMounts []string `json:"volumeMounts,omitempty"`
}

// ReadNamespacedState reads the LastUserSpecApplied annotation, which stores
// a JSON object keyed by concern name (e.g. "podExtensions", "auth"), and
// unmarshals the payload for the given concern into dst. dst is left
// untouched if the annotation is absent, malformed, or has no entry for
// concern.
//
// Namespacing by concern lets multiple independent ensure functions share
// one annotation without overwriting each other's tracked state: each
// concern only ever reads and writes its own key.
func ReadNamespacedState(obj client.Object, concern string, dst any) {
	raw := obj.GetAnnotations()[annotations.LastUserSpecApplied]
	if raw == "" {
		return
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &all); err != nil {
		return
	}
	payload, ok := all[concern]
	if !ok {
		return
	}
	_ = json.Unmarshal(payload, dst)
}

// WriteNamespacedState marshals src and stores it under concern in the
// LastUserSpecApplied annotation, re-reading the existing annotation first
// so other concerns' keys are preserved. If src marshals to an empty JSON
// object ("{}"), the concern's key is removed instead of stored. If no
// concerns remain, the annotation itself is removed.
func WriteNamespacedState(obj client.Object, concern string, src any) {
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = make(map[string]string)
	}

	all := map[string]json.RawMessage{}
	if raw, ok := anns[annotations.LastUserSpecApplied]; ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &all)
	}

	data, _ := json.Marshal(src)
	if string(data) == "{}" {
		delete(all, concern)
	} else {
		all[concern] = data
	}

	if len(all) == 0 {
		delete(anns, annotations.LastUserSpecApplied)
	} else {
		merged, _ := json.Marshal(all)
		anns[annotations.LastUserSpecApplied] = string(merged)
	}
	obj.SetAnnotations(anns)
}

// ReadLastApplied reads and parses the podExtensions tracking state from the
// object's LastUserSpecApplied annotation. Returns an empty state if absent
// or malformed.
func ReadLastApplied(obj client.Object) UserSpecState {
	var state UserSpecState
	ReadNamespacedState(obj, podExtensionsConcern, &state)
	return state
}

// WriteLastApplied serializes state under the podExtensions key of the
// LastUserSpecApplied annotation, preserving any other concern's data
// already stored under other keys in the same annotation.
func WriteLastApplied(obj client.Object, state UserSpecState) {
	WriteNamespacedState(obj, podExtensionsConcern, state)
}

// Names extracts names from a slice of items with a Name field via a getter function.
func Names[T any](items []T, name func(T) string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, name(item))
	}
	sort.Strings(names)
	return names
}

// StaleResources builds a partial PodSpec containing volumes and volume mounts
// that were tracked in prev but are absent from the current extensions. The
// result is intended as the Managed field for EnsureWithCleanup so that
// removeManaged cleans up stale items via reflect-based name matching.
func StaleResources(prev UserSpecState, ext v1.PodExtensions, containerName string) *core.PodSpec {
	currentVols := nameSet(ext.Volumes, func(v v1.AdditionalVolume) string { return v.Name })
	currentMounts := nameSet(ext.VolumeMounts, func(vm core.VolumeMount) string { return vm.Name })

	spec := &core.PodSpec{}

	for _, name := range prev.Volumes {
		if _, ok := currentVols[name]; !ok {
			spec.Volumes = append(spec.Volumes, core.Volume{Name: name})
		}
	}

	var staleMounts []core.VolumeMount
	for _, name := range prev.VolumeMounts {
		if _, ok := currentMounts[name]; !ok {
			staleMounts = append(staleMounts, core.VolumeMount{Name: name})
		}
	}
	if len(staleMounts) > 0 {
		spec.Containers = []core.Container{{
			Name:         containerName,
			VolumeMounts: staleMounts,
		}}
	}

	return spec
}

func nameSet[T any](items []T, name func(T) string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[name(item)] = struct{}{}
	}
	return set
}
