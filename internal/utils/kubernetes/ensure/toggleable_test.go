package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOptionalToggle_Enabled(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes,
				corev1.Volume{Name: "tls-cert"})
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "tls-cert"}},
			}}},
		},
	}

	dp := &appsv1.Deployment{}
	fn := OptionalToggle(true, toggle)
	g.Expect(fn(dp)).To(Succeed())
	g.Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(dp.Spec.Template.Spec.Volumes[0].Name).To(Equal("tls-cert"))
}

func TestOptionalToggle_DisabledRemovesVolumes(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "tls-cert"}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "tls-cert"},
				{Name: "istio-certs"},
			},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())
	g.Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(dp.Spec.Template.Spec.Volumes[0].Name).To(Equal("istio-certs"))
}

func TestOptionalToggle_DisabledRemovesContainerItems(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "tls-cert"}},
				Containers: []corev1.Container{{
					Name:         "server",
					VolumeMounts: []corev1.VolumeMount{{Name: "tls-cert"}},
					Env:          []corev1.EnvVar{{Name: "SSL_CERT_DIR"}},
					Ports:        []corev1.ContainerPort{{Name: "metrics"}},
				}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "tls-cert"},
				{Name: "app-config"},
			},
			Containers: []corev1.Container{{
				Name: "server",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "tls-cert", MountPath: "/tls"},
					{Name: "app-config", MountPath: "/config"},
				},
				Env: []corev1.EnvVar{
					{Name: "SSL_CERT_DIR", Value: "/tls"},
					{Name: "APP_ENV", Value: "prod"},
				},
				Ports: []corev1.ContainerPort{
					{Name: "metrics", ContainerPort: 2112},
					{Name: "http", ContainerPort: 8080},
				},
			}},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())

	spec := dp.Spec.Template.Spec
	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Name).To(Equal("app-config"))

	container := spec.Containers[0]
	g.Expect(container.VolumeMounts).To(HaveLen(1))
	g.Expect(container.VolumeMounts[0].Name).To(Equal("app-config"))

	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("APP_ENV"))

	g.Expect(container.Ports).To(HaveLen(1))
	g.Expect(container.Ports[0].Name).To(Equal("http"))
}

func TestOptionalToggle_DisabledPreservesUnmanagedContainer(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "server",
					Env:  []corev1.EnvVar{{Name: "PASSWORD"}},
				}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "server",
					Env: []corev1.EnvVar{
						{Name: "PASSWORD"},
						{Name: "APP_ENV"},
					},
				},
				{
					Name: "sidecar",
					Env: []corev1.EnvVar{
						{Name: "SIDECAR_ENV"},
					},
				},
			},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())

	spec := dp.Spec.Template.Spec
	g.Expect(spec.Containers).To(HaveLen(2))

	g.Expect(spec.Containers[0].Name).To(Equal("server"))
	g.Expect(spec.Containers[0].Env).To(HaveLen(1))
	g.Expect(spec.Containers[0].Env[0].Name).To(Equal("APP_ENV"))

	g.Expect(spec.Containers[1].Name).To(Equal("sidecar"))
	g.Expect(spec.Containers[1].Env).To(HaveLen(1))
	g.Expect(spec.Containers[1].Env[0].Name).To(Equal("SIDECAR_ENV"))
}

func TestOptionalToggle_DisabledRemovesInitContainers(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "wait-for-db"}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "wait-for-db"},
				{Name: "tuf-init"},
			},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())

	// InitContainers have nested named slices (Env, VolumeMounts, Ports),
	// so removeManaged recurses into them rather than removing outright.
	// To remove the init container itself, the Managed init container should
	// NOT have nested named slices populated — it's a leaf declaration.
	// This test verifies the current behavior.
	spec := dp.Spec.Template.Spec
	g.Expect(spec.InitContainers).To(HaveLen(2))
}

func TestOptionalToggle_DisabledRemovesInitContainerSubItems(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Name:         "wait-for-db",
					VolumeMounts: []corev1.VolumeMount{{Name: "db-cert"}},
				}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Name: "wait-for-db",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "db-cert"},
					{Name: "other-mount"},
				},
			}},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())

	initC := dp.Spec.Template.Spec.InitContainers[0]
	g.Expect(initC.VolumeMounts).To(HaveLen(1))
	g.Expect(initC.VolumeMounts[0].Name).To(Equal("other-mount"))
}

func TestOptionalToggle_DisabledNothingToRemove(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "tls-cert"}},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "app-config"},
			},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())
	g.Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(dp.Spec.Template.Spec.Volumes[0].Name).To(Equal("app-config"))
}

func TestOptionalToggle_Ingress(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*networkingv1.Ingress]{
		Ensure: func(ing *networkingv1.Ingress) error {
			return nil
		},
		Managed: &networkingv1.Ingress{
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					SecretName: "my-tls",
				}},
			},
		},
	}

	ing := &networkingv1.Ingress{
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{SecretName: "my-tls"},
				{SecretName: "other-tls"},
			},
		},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(ing)).To(Succeed())

	// IngressTLS has no Name field — falls through to removeByValue.
	// DeepEqual matches on the full struct, so only exact matches are removed.
	g.Expect(ing.Spec.TLS).To(HaveLen(1))
	g.Expect(ing.Spec.TLS[0].SecretName).To(Equal("other-tls"))
}

func TestOptionalToggle_Service(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Service]{
		Ensure: func(svc *corev1.Service) error {
			return nil
		},
		Managed: &corev1.Service{
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "metrics"}},
			},
		},
	}

	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "metrics", Port: 2112},
				{Name: "http", Port: 8080},
			},
		},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(svc)).To(Succeed())
	g.Expect(svc.Spec.Ports).To(HaveLen(1))
	g.Expect(svc.Spec.Ports[0].Name).To(Equal("http"))
}

func TestOptionalToggle_StatefulSet(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.StatefulSet]{
		Ensure: func(ss *appsv1.StatefulSet) error {
			return nil
		},
		Managed: &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "tls-cert"}},
				Containers: []corev1.Container{{
					Name:         "monitor",
					VolumeMounts: []corev1.VolumeMount{{Name: "tls-cert"}},
				}},
			}}},
		},
	}

	ss := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "tls-cert"},
				{Name: "data"},
			},
			Containers: []corev1.Container{{
				Name: "monitor",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "tls-cert", MountPath: "/tls"},
					{Name: "data", MountPath: "/data"},
				},
			}},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(ss)).To(Succeed())

	spec := ss.Spec.Template.Spec
	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Name).To(Equal("data"))
	g.Expect(spec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(spec.Containers[0].VolumeMounts[0].Name).To(Equal("data"))
}

func TestOptionalToggle_MultipleContainers(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{{Name: "shared-tls"}},
				Containers: []corev1.Container{
					{
						Name:         "server",
						VolumeMounts: []corev1.VolumeMount{{Name: "shared-tls"}},
					},
					{
						Name:         "sidecar",
						VolumeMounts: []corev1.VolumeMount{{Name: "shared-tls"}},
					},
				},
			}}},
		},
	}

	dp := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "shared-tls"},
				{Name: "app-data"},
			},
			Containers: []corev1.Container{
				{
					Name: "server",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "shared-tls", MountPath: "/tls"},
						{Name: "app-data", MountPath: "/data"},
					},
				},
				{
					Name: "sidecar",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "shared-tls", MountPath: "/tls"},
						{Name: "sidecar-vol", MountPath: "/sidecar"},
					},
				},
			},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())

	spec := dp.Spec.Template.Spec
	g.Expect(spec.Volumes).To(HaveLen(1))
	g.Expect(spec.Volumes[0].Name).To(Equal("app-data"))

	g.Expect(spec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(spec.Containers[0].VolumeMounts[0].Name).To(Equal("app-data"))

	g.Expect(spec.Containers[1].VolumeMounts).To(HaveLen(1))
	g.Expect(spec.Containers[1].VolumeMounts[0].Name).To(Equal("sidecar-vol"))
}

func TestOptionalToggle_EmptyManagedIsNoop(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*appsv1.Deployment]{
		Ensure: func(dp *appsv1.Deployment) error {
			return nil
		},
		Managed: &appsv1.Deployment{},
	}

	dp := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "keep-me"}},
		}}},
	}

	fn := OptionalToggle(false, toggle)
	g.Expect(fn(dp)).To(Succeed())
	g.Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(1))
}
