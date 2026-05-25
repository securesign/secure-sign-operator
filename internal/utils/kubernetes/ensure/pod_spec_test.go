package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
)

const testContainerName = "app"

func TestPodSecurityContext(t *testing.T) {
	tests := []struct {
		name   string
		spec   core.PodSpec
		verify func(Gomega, *core.PodSpec)
	}{
		{
			name: "sets all required fields on empty containers",
			spec: core.PodSpec{
				Containers:     []core.Container{{Name: testContainerName}},
				InitContainers: []core.Container{{Name: "init"}},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.SecurityContext).ToNot(BeNil())
				g.Expect(*spec.SecurityContext.RunAsNonRoot).To(BeTrue())
				g.Expect(spec.SecurityContext.SeccompProfile.Type).To(Equal(core.SeccompProfileTypeRuntimeDefault))

				for _, c := range append(spec.Containers, spec.InitContainers...) {
					g.Expect(c.SecurityContext).ToNot(BeNil())
					g.Expect(*c.SecurityContext.RunAsNonRoot).To(BeTrue())
					g.Expect(*c.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
					g.Expect(c.SecurityContext.Capabilities).ToNot(BeNil())
					g.Expect(c.SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")))
				}
			},
		},
		{
			name: "does not overwrite existing capabilities.drop",
			spec: core.PodSpec{
				Containers: []core.Container{{
					Name: testContainerName,
					SecurityContext: &core.SecurityContext{
						Capabilities: &core.Capabilities{
							Drop: []core.Capability{"NET_RAW"},
						},
					},
				}},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.Containers[0].SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("NET_RAW")))
			},
		},
		{
			name: "does not overwrite existing RunAsNonRoot on container",
			spec: core.PodSpec{
				Containers: []core.Container{{
					Name: testContainerName,
					SecurityContext: &core.SecurityContext{
						RunAsNonRoot: ptrBool(false),
					},
				}},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(*spec.Containers[0].SecurityContext.RunAsNonRoot).To(BeFalse())
				g.Expect(spec.Containers[0].SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")))
			},
		},
		{
			name: "handles multiple containers and init containers",
			spec: core.PodSpec{
				Containers:     []core.Container{{Name: "c1"}, {Name: "c2"}},
				InitContainers: []core.Container{{Name: "i1"}, {Name: "i2"}},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				for _, c := range spec.Containers {
					g.Expect(c.SecurityContext.Capabilities).ToNot(BeNil())
					g.Expect(c.SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")))
				}
				for _, c := range spec.InitContainers {
					g.Expect(c.SecurityContext.Capabilities).ToNot(BeNil())
					g.Expect(c.SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")))
				}
			},
		},
		{
			name: "handles empty pod spec",
			spec: core.PodSpec{},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.SecurityContext).ToNot(BeNil())
				g.Expect(*spec.SecurityContext.RunAsNonRoot).To(BeTrue())
				g.Expect(spec.SecurityContext.SeccompProfile.Type).To(Equal(core.SeccompProfileTypeRuntimeDefault))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := PodSecurityContext(&tt.spec)
			g.Expect(err).ToNot(HaveOccurred())
			tt.verify(g, &tt.spec)
		})
	}
}

func ptrBool(b bool) *bool {
	return &b
}
