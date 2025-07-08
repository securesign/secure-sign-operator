package kubernetes

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestRemoveVolumeByName(t *testing.T) {
	type args struct {
		podSpec    *corev1.PodSpec
		volumeName string
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *corev1.PodSpec)
	}{
		{
			name: "empty slice",
			args: args{
				podSpec:    &corev1.PodSpec{},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.BeEmpty())
			},
		},
		{
			name: "remove existing volume mount",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.HaveLen(1))
				g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal("volume2"))
			},
		},
		{
			name: "remove not existing volume mount",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume3",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.HaveLen(2))
				g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal("volume"))
				g.Expect(podSpec.Volumes[1].Name).To(gomega.Equal("volume2"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			RemoveVolumeByName(tt.args.podSpec, tt.args.volumeName)
			tt.verify(g, tt.args.podSpec)
		})
	}
}

func TestRemoveVolumeMountByName(t *testing.T) {
	type args struct {
		container  *corev1.Container
		volumeName string
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *corev1.Container)
	}{
		{
			name: "empty slice",
			args: args{
				container:  &corev1.Container{},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.BeEmpty())
			},
		},
		{
			name: "remove existing volume mount",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.HaveLen(1))
				g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal("volume2"))
			},
		},
		{
			name: "remove not existing volume mount",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume3",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.HaveLen(2))
				g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal("volume"))
				g.Expect(container.VolumeMounts[1].Name).To(gomega.Equal("volume2"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			RemoveVolumeMountByName(tt.args.container, tt.args.volumeName)
			tt.verify(g, tt.args.container)
		})
	}
}
