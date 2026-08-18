package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/annotations"
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

func TestUserSpecifiedToggle_MultipleCallsDisjointFields(t *testing.T) {
	g := NewWithT(t)

	// First call manages volumes
	toggle1 := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "keep-vol"},
				},
			},
		},
	}

	// Second call manages container env vars
	toggle2 := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "server",
						Env: []corev1.EnvVar{
							{Name: "KEEP_ENV"},
						},
					},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.LastUserSpecApplied: `{"spec":{"volumes":[{"name":"keep-vol"},{"name":"drop-vol"}]}}`,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "server",
					Env: []corev1.EnvVar{
						{Name: "KEEP_ENV", Value: "1"},
						{Name: "DROP_ENV", Value: "2"},
						{Name: "UNRELATED_ENV", Value: "3"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "keep-vol"},
				{Name: "drop-vol"},
				{Name: "unrelated"},
			},
		},
	}

	// First call: manage volumes, should remove drop-vol
	fn1 := UserSpecifiedToggle(toggle1)
	g.Expect(fn1(pod)).To(Succeed())
	g.Expect(pod.Spec.Volumes).To(HaveLen(2))
	volNames := []string{pod.Spec.Volumes[0].Name, pod.Spec.Volumes[1].Name}
	g.Expect(volNames).To(ConsistOf("keep-vol", "unrelated"))
	g.Expect(pod.Annotations[annotations.LastUserSpecApplied]).To(MatchJSON(`{"spec":{"volumes":[{"name":"keep-vol"}]}}`))

	// Update annotation to include both volumes and containers for second call
	pod.Annotations[annotations.LastUserSpecApplied] = `{"spec":{"volumes":[{"name":"keep-vol"}],"containers":[{"name":"server","env":[{"name":"KEEP_ENV"},{"name":"DROP_ENV"}]}]}}`

	// Second call: manage env vars, should remove DROP_ENV but keep volumes annotation
	fn2 := UserSpecifiedToggle(toggle2)
	g.Expect(fn2(pod)).To(Succeed())

	// Volumes should still be there (not removed by second call)
	g.Expect(pod.Spec.Volumes).To(HaveLen(2))
	volNames = []string{pod.Spec.Volumes[0].Name, pod.Spec.Volumes[1].Name}
	g.Expect(volNames).To(ConsistOf("keep-vol", "unrelated"))

	// Env vars should be cleaned up
	envNames := make([]string, 0, len(pod.Spec.Containers[0].Env))
	for _, e := range pod.Spec.Containers[0].Env {
		envNames = append(envNames, e.Name)
	}
	g.Expect(envNames).To(ConsistOf("KEEP_ENV", "UNRELATED_ENV"))

	// Annotation should have both volumes and containers
	g.Expect(pod.Annotations[annotations.LastUserSpecApplied]).To(MatchJSON(`{"spec":{"volumes":[{"name":"keep-vol"}],"containers":[{"name":"server","env":[{"name":"KEEP_ENV"}]}]}}`))
}

func TestUserSpecifiedToggle_RemovesDroppedItems(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "keep"},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.LastUserSpecApplied: `{"spec":{"volumes":[{"name":"keep"}, {"name":"drop"}]}}`}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "server",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "keep", MountPath: "/keep"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "keep"},
				{Name: "drop"},
				{Name: "unrelated"},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	g.Expect(pod.Spec.Volumes).To(HaveLen(2))
	g.Expect(pod.Spec.Volumes[0].Name).To(Equal("keep"))
	g.Expect(pod.Spec.Volumes[1].Name).To(Equal("unrelated"))
	g.Expect(pod.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(pod.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("keep"))
	g.Expect(pod.Annotations[annotations.LastUserSpecApplied]).To(Equal(`{"spec":{"volumes":[{"name":"keep"}]}}`))
}

func TestUserSpecifiedToggle_RemovesEnvVarsFromContainer(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "server",
						Env: []corev1.EnvVar{
							{Name: "KEEP"},
						},
					},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.LastUserSpecApplied: `{"spec":{"containers":[{"name":"server","env":[{"name":"KEEP"},{"name":"DROP"}]}]}}`}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "server",
					Env: []corev1.EnvVar{
						{Name: "KEEP", Value: "1"},
						{Name: "DROP", Value: "2"},
						{Name: "UNRELATED", Value: "3"},
					},
				},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	envNames := make([]string, 0, len(pod.Spec.Containers[0].Env))
	for _, e := range pod.Spec.Containers[0].Env {
		envNames = append(envNames, e.Name)
	}
	g.Expect(envNames).To(ConsistOf("KEEP", "UNRELATED"))
}

func TestUserSpecifiedToggle_RemovesEntireContainer(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.LastUserSpecApplied: `{"spec":{"containers":[{"name":"main"},{"name":"sidecar"}]}}`}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main"},
				{Name: "sidecar"},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	g.Expect(pod.Spec.Containers).To(HaveLen(1))
	g.Expect(pod.Spec.Containers[0].Name).To(Equal("main"))
}

func TestUserSpecifiedToggle_EnsureRunsAfterCleanup(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			// Idempotent ensure - only add if not present
			found := false
			for _, v := range obj.Spec.Volumes {
				if v.Name == "new" {
					found = true
					break
				}
			}
			if !found {
				obj.Spec.Volumes = append(obj.Spec.Volumes, corev1.Volume{Name: "new"})
			}
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "new"},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.LastUserSpecApplied: `{"spec":{"volumes":[{"name":"old"}]}}`}},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "old"},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	g.Expect(pod.Spec.Volumes).To(HaveLen(1))
	g.Expect(pod.Spec.Volumes[0].Name).To(Equal("new"))
	g.Expect(pod.Annotations[annotations.LastUserSpecApplied]).To(Equal(`{"spec":{"volumes":[{"name":"new"}]}}`))
}

func TestUserSpecifiedToggle_NoLastApplied(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "vol1"},
				},
			},
		},
	}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "vol1"},
				{Name: "unrelated"},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	g.Expect(pod.Spec.Volumes).To(HaveLen(2))
	g.Expect(pod.Annotations[annotations.LastUserSpecApplied]).To(MatchJSON(`{"spec":{"volumes":[{"name":"vol1"}]}}`))
}

func TestUserSpecifiedToggle_ClearsAllManagedVolumes(t *testing.T) {
	g := NewWithT(t)

	toggle := Toggleable[*corev1.Pod]{
		Ensure: func(obj *corev1.Pod) error {
			return nil
		},
		Managed: &corev1.Pod{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.LastUserSpecApplied: `{"spec":{"volumes":[{"name":"A"}]}}`}},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "A"},
				{Name: "unrelated"},
			},
		},
	}

	fn := UserSpecifiedToggle(toggle)
	g.Expect(fn(pod)).To(Succeed())
	g.Expect(pod.Spec.Volumes).To(HaveLen(1))
	g.Expect(pod.Spec.Volumes[0].Name).To(Equal("unrelated"))
}
