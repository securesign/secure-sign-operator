package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/config"
	core "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
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

// TestPodSecurityContext_Kubernetes verifies runAsUser / fsGroup are set when
// not on OpenShift and cleared when on OpenShift.
func TestPodSecurityContext_PlatformMode(t *testing.T) {
	origOpenshift := config.Openshift
	t.Cleanup(func() { config.Openshift = origOpenshift })

	t.Run("kubernetes mode sets fsGroup and runAsUser when nil", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		spec := core.PodSpec{
			Containers:     []core.Container{{Name: testContainerName}},
			InitContainers: []core.Container{{Name: "init"}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(1001))))
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))))
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))))
	})

	t.Run("kubernetes mode does not overwrite existing fsGroup or runAsUser", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		spec := core.PodSpec{
			SecurityContext: &core.PodSecurityContext{FSGroup: ptr.To(int64(2000))},
			Containers: []core.Container{{
				Name:            testContainerName,
				SecurityContext: &core.SecurityContext{RunAsUser: ptr.To(int64(2000))},
			}},
			InitContainers: []core.Container{{
				Name:            "init",
				SecurityContext: &core.SecurityContext{RunAsUser: ptr.To(int64(2000))},
			}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(2000))))
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(2000))))
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(HaveValue(Equal(int64(2000))))
	})

	t.Run("openshift mode does not set fsGroup or runAsUser", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		spec := core.PodSpec{
			Containers:     []core.Container{{Name: testContainerName}},
			InitContainers: []core.Container{{Name: "init"}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(BeNil())
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(BeNil())
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(BeNil())
	})

	t.Run("openshift mode clears stale fsGroup and runAsUser", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		spec := core.PodSpec{
			SecurityContext: &core.PodSecurityContext{FSGroup: ptr.To(int64(1001))},
			Containers: []core.Container{{
				Name: testContainerName,
				SecurityContext: &core.SecurityContext{
					RunAsUser: ptr.To(int64(1001)),
				},
			}},
			InitContainers: []core.Container{{
				Name: "init",
				SecurityContext: &core.SecurityContext{
					RunAsUser: ptr.To(int64(1001)),
				},
			}},
		}
		g.Expect(PodSecurityContext(&spec)).To(Succeed())
		g.Expect(spec.SecurityContext.FSGroup).To(BeNil())
		g.Expect(spec.Containers[0].SecurityContext.RunAsUser).To(BeNil())
		g.Expect(spec.InitContainers[0].SecurityContext.RunAsUser).To(BeNil())
	})
}

func ptrBool(b bool) *bool {
	return &b
}

func TestReconcileUserPodResources(t *testing.T) {
	findContainer := func(spec *core.PodSpec, name string) *core.Container {
		for i := range spec.Containers {
			if spec.Containers[i].Name == name {
				return &spec.Containers[i]
			}
		}
		return nil
	}

	tests := []struct {
		name    string
		podSpec core.PodSpec
		ext     rhtasv1.PodExtensions
		prev    UserSpecState
		verify  func(Gomega, *core.PodSpec, UserSpecState)
	}{
		{
			name:    "adds volumes and mounts from empty state",
			podSpec: core.PodSpec{Containers: []core.Container{{Name: "app"}}},
			ext: rhtasv1.PodExtensions{
				Volumes: []rhtasv1.AdditionalVolume{
					{Name: "custom-config", AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{LocalObjectReference: core.LocalObjectReference{Name: "my-cm"}},
					}},
				},
				VolumeMounts: []core.VolumeMount{
					{Name: "custom-config", MountPath: "/etc/custom"},
				},
			},
			prev: UserSpecState{},
			verify: func(g Gomega, spec *core.PodSpec, current UserSpecState) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].Name).To(Equal("custom-config"))
				g.Expect(spec.Volumes[0].ConfigMap.Name).To(Equal("my-cm"))
				c := findContainer(spec, "app")
				g.Expect(c.VolumeMounts).To(HaveLen(1))
				g.Expect(c.VolumeMounts[0].MountPath).To(Equal("/etc/custom"))
				g.Expect(current.Volumes).To(ConsistOf("custom-config"))
				g.Expect(current.VolumeMounts).To(ConsistOf("custom-config"))
			},
		},
		{
			name: "removes stale volumes and mounts",
			podSpec: core.PodSpec{
				Containers: []core.Container{{
					Name: "app",
					VolumeMounts: []core.VolumeMount{
						{Name: "stale-vol", MountPath: "/old"},
						{Name: "operator-vol", MountPath: "/op"},
					},
				}},
				Volumes: []core.Volume{
					{Name: "stale-vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
					{Name: "operator-vol", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
				},
			},
			ext:  rhtasv1.PodExtensions{},
			prev: UserSpecState{Volumes: []string{"stale-vol"}, VolumeMounts: []string{"stale-vol"}},
			verify: func(g Gomega, spec *core.PodSpec, current UserSpecState) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].Name).To(Equal("operator-vol"))
				c := findContainer(spec, "app")
				g.Expect(c.VolumeMounts).To(HaveLen(1))
				g.Expect(c.VolumeMounts[0].Name).To(Equal("operator-vol"))
				g.Expect(current.Volumes).To(BeEmpty())
				g.Expect(current.VolumeMounts).To(BeEmpty())
			},
		},
		{
			name: "does not remove volumes not in previous state",
			podSpec: core.PodSpec{
				Containers: []core.Container{{
					Name:         "app",
					VolumeMounts: []core.VolumeMount{{Name: "operator-managed", MountPath: "/op"}},
				}},
				Volumes: []core.Volume{
					{Name: "operator-managed", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
				},
			},
			ext:  rhtasv1.PodExtensions{},
			prev: UserSpecState{},
			verify: func(g Gomega, spec *core.PodSpec, current UserSpecState) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].Name).To(Equal("operator-managed"))
				c := findContainer(spec, "app")
				g.Expect(c.VolumeMounts).To(HaveLen(1))
			},
		},
		{
			name: "updates existing volume source",
			podSpec: core.PodSpec{
				Containers: []core.Container{{Name: "app"}},
				Volumes: []core.Volume{
					{Name: "cfg", VolumeSource: core.VolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{LocalObjectReference: core.LocalObjectReference{Name: "old-cm"}},
					}},
				},
			},
			ext: rhtasv1.PodExtensions{
				Volumes: []rhtasv1.AdditionalVolume{
					{Name: "cfg", AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{LocalObjectReference: core.LocalObjectReference{Name: "new-cm"}},
					}},
				},
			},
			prev: UserSpecState{Volumes: []string{"cfg"}},
			verify: func(g Gomega, spec *core.PodSpec, current UserSpecState) {
				g.Expect(spec.Volumes).To(HaveLen(1))
				g.Expect(spec.Volumes[0].ConfigMap.Name).To(Equal("new-cm"))
				g.Expect(current.Volumes).To(ConsistOf("cfg"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			current, err := ReconcileUserPodResources(&tt.podSpec, "app", tt.ext, tt.prev)
			g.Expect(err).ToNot(HaveOccurred())
			tt.verify(g, &tt.podSpec, current)
		})
	}
}

func TestReconcileInitContainers(t *testing.T) {
	tests := []struct {
		name    string
		podSpec core.PodSpec
		specs   []rhtasv1.InitContainerSpec
		verify  func(Gomega, *core.PodSpec)
	}{
		{
			name:    "basic reconciliation adds init container",
			podSpec: core.PodSpec{},
			specs: []rhtasv1.InitContainerSpec{
				{
					Name:    "setup-init",
					Image:   "vendor-init:latest",
					Command: []string{"/bin/setup"},
				},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.InitContainers).To(HaveLen(1))
				g.Expect(spec.InitContainers[0].Name).To(Equal("setup-init"))
				g.Expect(spec.InitContainers[0].Image).To(Equal("vendor-init:latest"))
				g.Expect(spec.InitContainers[0].Command).To(Equal([]string{"/bin/setup"}))
				g.Expect(spec.InitContainers[0].VolumeMounts).To(BeEmpty())
			},
		},
		{
			name:    "empty specs clears init containers",
			podSpec: core.PodSpec{InitContainers: []core.Container{{Name: "old"}}},
			specs:   []rhtasv1.InitContainerSpec{},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.InitContainers).To(BeNil())
			},
		},
		{
			name: "preserves desired init containers and removes stale ones",
			podSpec: core.PodSpec{
				InitContainers: []core.Container{
					{Name: "stale-container", Image: "old:1.0"},
					{Name: "keep-me", Image: "keep:1.0"},
				},
			},
			specs: []rhtasv1.InitContainerSpec{
				{
					Name:  "keep-me",
					Image: "keep:2.0",
				},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.InitContainers).To(HaveLen(1))
				g.Expect(spec.InitContainers[0].Name).To(Equal("keep-me"))
				g.Expect(spec.InitContainers[0].Image).To(Equal("keep:2.0"))
			},
		},
		{
			name:    "user-defined volume mounts are preserved",
			podSpec: core.PodSpec{},
			specs: []rhtasv1.InitContainerSpec{
				{
					Name:  "setup",
					Image: "setup:latest",
					VolumeMounts: []core.VolumeMount{
						{Name: "custom-config", MountPath: "/etc/custom"},
					},
				},
			},
			verify: func(g Gomega, spec *core.PodSpec) {
				g.Expect(spec.InitContainers).To(HaveLen(1))
				mounts := spec.InitContainers[0].VolumeMounts
				g.Expect(mounts).To(HaveLen(1))
				g.Expect(mounts[0].Name).To(Equal("custom-config"))
				g.Expect(mounts[0].MountPath).To(Equal("/etc/custom"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ReconcileInitContainers(&tt.podSpec, tt.specs)
			tt.verify(g, &tt.podSpec)
		})
	}
}
