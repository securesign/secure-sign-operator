package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/annotations"
	corev1 "k8s.io/api/core/v1"
)

func TestSetGodebugEnv(t *testing.T) {
	tests := []struct {
		name                 string
		godebug              string
		componentAnnotations map[string]string
		containers           []corev1.Container
		verify               func(Gomega, []corev1.Container)
	}{
		{
			name:                 "set GODEBUG on single container",
			godebug:              "fips140=only",
			componentAnnotations: nil,
			containers: []corev1.Container{
				{Name: "server", Env: []corev1.EnvVar{{Name: "EXISTING", Value: "value"}}},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(2))
				g.Expect(containers[0].Env).Should(ContainElement(corev1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}))
				g.Expect(containers[0].Env).Should(ContainElement(corev1.EnvVar{Name: "EXISTING", Value: "value"}))
			},
		},
		{
			name:                 "no-op when GODEBUG is not set and no existing GODEBUG",
			godebug:              "",
			componentAnnotations: nil,
			containers: []corev1.Container{
				{Name: "server", Env: []corev1.EnvVar{{Name: "EXISTING", Value: "value"}}},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0].Name).Should(Equal("EXISTING"))
			},
		},
		{
			name:                 "removes stale GODEBUG when operator env is cleared",
			godebug:              "",
			componentAnnotations: nil,
			containers: []corev1.Container{
				{Name: "server", Env: []corev1.EnvVar{{Name: "GODEBUG", Value: "fips140=only"}, {Name: "EXISTING", Value: "value"}}},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0].Name).Should(Equal("EXISTING"))
			},
		},
		{
			name:                 "set GODEBUG on multiple containers",
			godebug:              "fips140=only",
			componentAnnotations: nil,
			containers: []corev1.Container{
				{Name: "server"},
				{Name: "sidecar"},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				for _, c := range containers {
					g.Expect(c.Env).Should(HaveLen(1))
					g.Expect(c.Env[0]).Should(Equal(corev1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}))
				}
			},
		},
		{
			name:                 "idempotent - no duplication on repeat call",
			godebug:              "fips140=only",
			componentAnnotations: nil,
			containers: []corev1.Container{
				{Name: "server"},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				SetGodebugEnv(containers, nil)
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0]).Should(Equal(corev1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}))
			},
		},
		{
			name:    "annotation overrides operator env",
			godebug: "fips140=only",
			componentAnnotations: map[string]string{
				annotations.Godebug: "fips140=on",
			},
			containers: []corev1.Container{
				{Name: "server"},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0]).Should(Equal(corev1.EnvVar{Name: "GODEBUG", Value: "fips140=on"}))
			},
		},
		{
			name:    "empty annotation disables propagation and removes existing GODEBUG",
			godebug: "fips140=only",
			componentAnnotations: map[string]string{
				annotations.Godebug: "",
			},
			containers: []corev1.Container{
				{Name: "server", Env: []corev1.EnvVar{{Name: "GODEBUG", Value: "fips140=only"}, {Name: "EXISTING", Value: "value"}}},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0].Name).Should(Equal("EXISTING"))
			},
		},
		{
			name:    "annotation without operator env",
			godebug: "",
			componentAnnotations: map[string]string{
				annotations.Godebug: "fips140=on",
			},
			containers: []corev1.Container{
				{Name: "server"},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				g.Expect(containers[0].Env).Should(HaveLen(1))
				g.Expect(containers[0].Env[0]).Should(Equal(corev1.EnvVar{Name: "GODEBUG", Value: "fips140=on"}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Setenv("GODEBUG", tt.godebug)
			SetGodebugEnv(tt.containers, tt.componentAnnotations)
			tt.verify(g, tt.containers)
		})
	}
}
