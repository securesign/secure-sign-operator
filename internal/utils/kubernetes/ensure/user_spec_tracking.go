package ensure

import (
	"encoding/json"
	"slices"
	"sort"

	v1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UserSpecState tracks the names of user-specified resources that were applied
// during the last reconcile. Stored as JSON in the LastUserSpecApplied annotation.
type UserSpecState struct {
	Volumes      []string `json:"volumes,omitempty"`
	VolumeMounts []string `json:"volumeMounts,omitempty"`
	Annotations  []string `json:"annotations,omitempty"`
	Labels       []string `json:"labels,omitempty"`
}

// Equal reports whether two UserSpecState values are identical.
func (s UserSpecState) Equal(other UserSpecState) bool {
	return slices.Equal(s.Volumes, other.Volumes) &&
		slices.Equal(s.VolumeMounts, other.VolumeMounts) &&
		slices.Equal(s.Annotations, other.Annotations) &&
		slices.Equal(s.Labels, other.Labels)
}

// ReadLastApplied reads and parses the tracking annotation from the object.
// Returns an empty state if the annotation is absent or malformed.
func ReadLastApplied(obj client.Object) UserSpecState {
	raw := obj.GetAnnotations()[annotations.LastUserSpecApplied]
	if raw == "" {
		return UserSpecState{}
	}
	var state UserSpecState
	_ = json.Unmarshal([]byte(raw), &state)
	return state
}

// WriteLastApplied serializes the current state into the tracking annotation.
// If the state is empty, the annotation is removed.
func WriteLastApplied(obj client.Object, state UserSpecState) {
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = make(map[string]string)
	}

	if state.Equal(UserSpecState{}) {
		delete(anns, annotations.LastUserSpecApplied)
	} else {
		data, _ := json.Marshal(state)
		anns[annotations.LastUserSpecApplied] = string(data)
	}
	obj.SetAnnotations(anns)
}

// ManagedKeys returns the union of previous and desired key sets.
// Pass the result to ensure.Annotations or ensure.Labels so that keys removed
// from the desired set are still in the managed list and get deleted.
func ManagedKeys(previous, desired []string) []string {
	seen := make(map[string]struct{}, len(previous)+len(desired))
	for _, k := range previous {
		seen[k] = struct{}{}
	}
	for _, k := range desired {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// KeySet extracts the keys from a map and returns them sorted.
func KeySet(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
