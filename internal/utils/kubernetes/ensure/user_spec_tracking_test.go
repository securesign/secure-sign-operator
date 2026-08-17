package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadLastApplied(t *testing.T) {
	tests := []struct {
		name   string
		ann    map[string]string
		expect UserSpecState
	}{
		{
			name:   "no annotation returns empty state",
			ann:    nil,
			expect: UserSpecState{},
		},
		{
			name:   "empty string returns empty state",
			ann:    map[string]string{annotations.LastUserSpecApplied: ""},
			expect: UserSpecState{},
		},
		{
			name: "valid JSON is parsed",
			ann: map[string]string{
				annotations.LastUserSpecApplied: `{"volumes":["vol-a","vol-b"],"volumeMounts":["vol-a"]}`,
			},
			expect: UserSpecState{
				Volumes:      []string{"vol-a", "vol-b"},
				VolumeMounts: []string{"vol-a"},
			},
		},
		{
			name:   "malformed JSON returns empty state",
			ann:    map[string]string{annotations.LastUserSpecApplied: `not json`},
			expect: UserSpecState{},
		},
		{
			name: "annotations and labels are parsed",
			ann: map[string]string{
				annotations.LastUserSpecApplied: `{"annotations":["k1"],"labels":["l1","l2"]}`,
			},
			expect: UserSpecState{
				Annotations: []string{"k1"},
				Labels:      []string{"l1", "l2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: tt.ann}}
			got := ReadLastApplied(svc)
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func TestWriteLastApplied(t *testing.T) {
	t.Run("writes state to annotation", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{}}
		state := UserSpecState{Volumes: []string{"vol-a"}, VolumeMounts: []string{"vol-a"}}
		WriteLastApplied(svc, state)
		g.Expect(svc.GetAnnotations()).To(HaveKey(annotations.LastUserSpecApplied))

		roundtrip := ReadLastApplied(svc)
		g.Expect(roundtrip).To(Equal(state))
	})

	t.Run("empty state removes annotation", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"volumes":["old"]}`,
				"other-annotation":              "keep",
			},
		}}
		WriteLastApplied(svc, UserSpecState{})
		g.Expect(svc.GetAnnotations()).ToNot(HaveKey(annotations.LastUserSpecApplied))
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue("other-annotation", "keep"))
	})

	t.Run("preserves existing annotations", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"existing": "value"},
		}}
		WriteLastApplied(svc, UserSpecState{Labels: []string{"l1"}})
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue("existing", "value"))
		g.Expect(svc.GetAnnotations()).To(HaveKey(annotations.LastUserSpecApplied))
	})
}

func TestManagedKeys(t *testing.T) {
	tests := []struct {
		name     string
		previous []string
		desired  []string
		expect   []string
	}{
		{
			name:     "union of previous and desired",
			previous: []string{"a", "b"},
			desired:  []string{"b", "c"},
			expect:   []string{"a", "b", "c"},
		},
		{
			name:     "empty previous",
			previous: nil,
			desired:  []string{"a"},
			expect:   []string{"a"},
		},
		{
			name:     "empty desired includes previous for cleanup",
			previous: []string{"a", "b"},
			desired:  nil,
			expect:   []string{"a", "b"},
		},
		{
			name:     "both empty",
			previous: nil,
			desired:  nil,
			expect:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := ManagedKeys(tt.previous, tt.desired)
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func TestKeySet(t *testing.T) {
	g := NewWithT(t)
	g.Expect(KeySet(map[string]string{"b": "2", "a": "1"})).To(Equal([]string{"a", "b"}))
	g.Expect(KeySet(nil)).To(Equal([]string{}))
}

func TestUserSpecState_Equal(t *testing.T) {
	tests := []struct {
		name  string
		a, b  UserSpecState
		equal bool
	}{
		{
			name:  "both empty",
			a:     UserSpecState{},
			b:     UserSpecState{},
			equal: true,
		},
		{
			name:  "same volumes",
			a:     UserSpecState{Volumes: []string{"a", "b"}},
			b:     UserSpecState{Volumes: []string{"a", "b"}},
			equal: true,
		},
		{
			name:  "different volumes",
			a:     UserSpecState{Volumes: []string{"a"}},
			b:     UserSpecState{Volumes: []string{"b"}},
			equal: false,
		},
		{
			name:  "nil and empty slice are equal",
			a:     UserSpecState{Volumes: nil},
			b:     UserSpecState{Volumes: []string{}},
			equal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.a.Equal(tt.b)).To(Equal(tt.equal))
		})
	}
}

func TestStaleResources(t *testing.T) {
	tests := []struct {
		name          string
		prev          UserSpecState
		ext           rhtasv1.PodExtensions
		containerName string
		verify        func(Gomega, *v1.PodSpec)
	}{
		{
			name:          "stale volumes and mounts are included",
			prev:          UserSpecState{Volumes: []string{"a", "b"}, VolumeMounts: []string{"a", "b"}},
			ext:           rhtasv1.PodExtensions{Volumes: []rhtasv1.AdditionalVolume{{Name: "b"}}, VolumeMounts: []v1.VolumeMount{{Name: "b"}}},
			containerName: "app",
			verify: func(g Gomega, spec *v1.PodSpec) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].Name).To(Equal("a"))
				g.Expect(spec.Containers).To(HaveLen(1))
				g.Expect(spec.Containers[0].Name).To(Equal("app"))
				g.Expect(spec.Containers[0].VolumeMounts).To(HaveLen(1))
				g.Expect(spec.Containers[0].VolumeMounts[0].Name).To(Equal("a"))
			},
		},
		{
			name:          "no stale items returns empty spec",
			prev:          UserSpecState{Volumes: []string{"a"}, VolumeMounts: []string{"a"}},
			ext:           rhtasv1.PodExtensions{Volumes: []rhtasv1.AdditionalVolume{{Name: "a"}}, VolumeMounts: []v1.VolumeMount{{Name: "a"}}},
			containerName: "app",
			verify: func(g Gomega, spec *v1.PodSpec) {
				g.Expect(spec.Volumes).To(BeEmpty())
				g.Expect(spec.Containers).To(BeEmpty())
			},
		},
		{
			name:          "empty prev returns empty spec",
			prev:          UserSpecState{},
			ext:           rhtasv1.PodExtensions{Volumes: []rhtasv1.AdditionalVolume{{Name: "a"}}},
			containerName: "app",
			verify: func(g Gomega, spec *v1.PodSpec) {
				g.Expect(spec.Volumes).To(BeEmpty())
				g.Expect(spec.Containers).To(BeEmpty())
			},
		},
		{
			name:          "stale volumes only, no mounts",
			prev:          UserSpecState{Volumes: []string{"old"}},
			ext:           rhtasv1.PodExtensions{},
			containerName: "app",
			verify: func(g Gomega, spec *v1.PodSpec) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].Name).To(Equal("old"))
				g.Expect(spec.Containers).To(BeEmpty())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := StaleResources(tt.prev, tt.ext, tt.containerName)
			tt.verify(g, got)
		})
	}
}
