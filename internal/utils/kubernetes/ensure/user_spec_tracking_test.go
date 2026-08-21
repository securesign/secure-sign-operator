package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadNamespacedState(t *testing.T) {
	tests := []struct {
		name    string
		ann     map[string]string
		concern string
		expect  UserSpecState
	}{
		{
			name:    "no annotation leaves dst zero value",
			ann:     nil,
			concern: "podExtensions",
			expect:  UserSpecState{},
		},
		{
			name:    "concern present is parsed",
			ann:     map[string]string{annotations.LastUserSpecApplied: `{"podExtensions":{"volumes":["a"]}}`},
			concern: "podExtensions",
			expect:  UserSpecState{Volumes: []string{"a"}},
		},
		{
			name:    "other concern's key is ignored",
			ann:     map[string]string{annotations.LastUserSpecApplied: `{"auth":{"volumes":["ignore-me"]}}`},
			concern: "podExtensions",
			expect:  UserSpecState{},
		},
		{
			name:    "malformed annotation leaves dst zero value",
			ann:     map[string]string{annotations.LastUserSpecApplied: `not json`},
			concern: "podExtensions",
			expect:  UserSpecState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: tt.ann}}
			var got UserSpecState
			ReadNamespacedState(svc, tt.concern, &got)
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func TestWriteNamespacedState(t *testing.T) {
	t.Run("writes concern under its own key", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{}}
		WriteNamespacedState(svc, "podExtensions", UserSpecState{Volumes: []string{"a"}})
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue(annotations.LastUserSpecApplied, `{"podExtensions":{"volumes":["a"]}}`))
	})

	t.Run("preserves another concern's key already present", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"auth":{"volumes":["auth-vol"]}}`,
			},
		}}
		WriteNamespacedState(svc, "podExtensions", UserSpecState{Volumes: []string{"a"}})

		var auth UserSpecState
		ReadNamespacedState(svc, "auth", &auth)
		g.Expect(auth).To(Equal(UserSpecState{Volumes: []string{"auth-vol"}}))

		var podExt UserSpecState
		ReadNamespacedState(svc, "podExtensions", &podExt)
		g.Expect(podExt).To(Equal(UserSpecState{Volumes: []string{"a"}}))
	})

	t.Run("removing this concern's data preserves another concern's key", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"auth":{"volumes":["auth-vol"]},"podExtensions":{"volumes":["old"]}}`,
			},
		}}
		WriteNamespacedState(svc, "podExtensions", UserSpecState{})

		g.Expect(svc.GetAnnotations()[annotations.LastUserSpecApplied]).To(Equal(`{"auth":{"volumes":["auth-vol"]}}`))
	})

	t.Run("removing the last concern removes the whole annotation", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"podExtensions":{"volumes":["old"]}}`,
				"other-annotation":              "keep",
			},
		}}
		WriteNamespacedState(svc, "podExtensions", UserSpecState{})

		g.Expect(svc.GetAnnotations()).ToNot(HaveKey(annotations.LastUserSpecApplied))
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue("other-annotation", "keep"))
	})
}

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
				annotations.LastUserSpecApplied: `{"podExtensions":{"volumes":["vol-a","vol-b"],"volumeMounts":["vol-a"]}}`,
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
			name: "another concern's data does not leak into podExtensions state",
			ann: map[string]string{
				annotations.LastUserSpecApplied: `{"auth":{"volumes":["auth-vol"]}}`,
			},
			expect: UserSpecState{},
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
	t.Run("writes state under the podExtensions key", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{}}
		state := UserSpecState{Volumes: []string{"vol-a"}, VolumeMounts: []string{"vol-a"}}
		WriteLastApplied(svc, state)
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue(annotations.LastUserSpecApplied,
			`{"podExtensions":{"volumes":["vol-a"],"volumeMounts":["vol-a"]}}`))

		roundtrip := ReadLastApplied(svc)
		g.Expect(roundtrip).To(Equal(state))
	})

	t.Run("empty state removes only the podExtensions key", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"auth":{"volumes":["auth-vol"]},"podExtensions":{"volumes":["old"]}}`,
				"other-annotation":              "keep",
			},
		}}
		WriteLastApplied(svc, UserSpecState{})
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue(annotations.LastUserSpecApplied, `{"auth":{"volumes":["auth-vol"]}}`))
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue("other-annotation", "keep"))
	})

	t.Run("preserves existing annotations", func(t *testing.T) {
		g := NewWithT(t)
		svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"existing": "value"},
		}}
		WriteLastApplied(svc, UserSpecState{Volumes: []string{"a"}})
		g.Expect(svc.GetAnnotations()).To(HaveKeyWithValue("existing", "value"))
		g.Expect(svc.GetAnnotations()).To(HaveKey(annotations.LastUserSpecApplied))
	})
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
