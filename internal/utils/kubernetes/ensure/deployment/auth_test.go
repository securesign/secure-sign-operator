package deployment

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAuth_RemovesDeletedEnvVars(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "server",
							Env: []corev1.EnvVar{
								{Name: "UNRELATED", Value: "should-stay"},
							},
						},
					},
				},
			},
		},
	}

	// Reconcile 1: Add 2 auth env vars
	auth1 := &rhtasv1.Auth{
		Env: []corev1.EnvVar{
			{Name: "AUTH_USER", Value: "admin"},
			{Name: "AUTH_PASS", Value: "secret"},
		},
	}

	fn1 := Auth("server", auth1)
	g.Expect(fn1(deployment)).To(Succeed())

	container := deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(3)) // UNRELATED, AUTH_USER, AUTH_PASS
	g.Expect(deployment.Annotations[annotations.LastUserSpecApplied]).ToNot(BeEmpty())

	// Reconcile 2: Remove AUTH_PASS from spec
	auth2 := &rhtasv1.Auth{
		Env: []corev1.EnvVar{
			{Name: "AUTH_USER", Value: "admin"},
			// AUTH_PASS removed
		},
	}

	fn2 := Auth("server", auth2)
	g.Expect(fn2(deployment)).To(Succeed())

	// Verify AUTH_PASS was removed, UNRELATED stayed
	container = deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(2)) // UNRELATED, AUTH_USER
	envNames := []string{container.Env[0].Name, container.Env[1].Name}
	g.Expect(envNames).To(ConsistOf("AUTH_USER", "UNRELATED"))
}

func TestAuth_RemovesVolumesAndMounts(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "server"}},
				},
			},
		},
	}

	// Reconcile 1: Add auth with secrets
	auth1 := &rhtasv1.Auth{
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "auth-secret"}},
		},
	}

	fn1 := Auth("server", auth1)
	g.Expect(fn1(deployment)).To(Succeed())

	// Verify volume and mount added
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal("auth"))
	g.Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(1))

	// Reconcile 2: Remove secrets from spec
	auth2 := &rhtasv1.Auth{
		// SecretMount removed
	}

	fn2 := Auth("server", auth2)
	g.Expect(fn2(deployment)).To(Succeed())

	// Verify volume and mount were removed
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(BeEmpty())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(BeEmpty())
}

func TestAuth_NilAuthClearsEverything(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "server",
							Env:  []corev1.EnvVar{{Name: "KEEP_ME", Value: "unrelated"}},
						},
					},
				},
			},
		},
	}

	// Reconcile 1: Add full auth config
	auth1 := &rhtasv1.Auth{
		Env: []corev1.EnvVar{
			{Name: "AUTH_TOKEN", Value: "xyz"},
		},
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "creds"}},
		},
	}

	fn1 := Auth("server", auth1)
	g.Expect(fn1(deployment)).To(Succeed())

	container := deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(2)) // KEEP_ME, AUTH_TOKEN
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(container.VolumeMounts).To(HaveLen(1))

	// Reconcile 2: Set auth to nil
	fn2 := Auth("server", nil)
	g.Expect(fn2(deployment)).To(Succeed())

	// Verify all auth resources removed, unrelated kept
	container = deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("KEEP_ME"))
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(BeEmpty())
	g.Expect(container.VolumeMounts).To(BeEmpty())
}

func TestAuth_MultipleReconcileCycles(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "server"}},
				},
			},
		},
	}

	// Cycle 1: Add env var A
	g.Expect(Auth("server", &rhtasv1.Auth{
		Env: []corev1.EnvVar{{Name: "A", Value: "1"}},
	})(deployment)).To(Succeed())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(1))

	// Cycle 2: Add env var B
	g.Expect(Auth("server", &rhtasv1.Auth{
		Env: []corev1.EnvVar{
			{Name: "A", Value: "1"},
			{Name: "B", Value: "2"},
		},
	})(deployment)).To(Succeed())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(2))

	// Cycle 3: Remove A, keep B
	g.Expect(Auth("server", &rhtasv1.Auth{
		Env: []corev1.EnvVar{{Name: "B", Value: "2"}},
	})(deployment)).To(Succeed())
	container := deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("B"))

	// Cycle 4: Clear all
	g.Expect(Auth("server", &rhtasv1.Auth{})(deployment)).To(Succeed())
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(BeEmpty())

	// Cycle 5: Add C (verifies we can add after clearing)
	g.Expect(Auth("server", &rhtasv1.Auth{
		Env: []corev1.EnvVar{{Name: "C", Value: "3"}},
	})(deployment)).To(Succeed())
	container = deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("C"))
}

func TestAuth_PreservesUnrelatedResources(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "server",
							Env: []corev1.EnvVar{
								{Name: "APP_ENV", Value: "prod"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "app-data", MountPath: "/data"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "app-data"},
					},
				},
			},
		},
	}

	// Add auth resources
	g.Expect(Auth("server", &rhtasv1.Auth{
		Env: []corev1.EnvVar{{Name: "AUTH_KEY", Value: "secret"}},
		SecretMount: []rhtasv1.SecretKeySelector{
			{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "auth-creds"}},
		},
	})(deployment)).To(Succeed())

	container := deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(2))          // APP_ENV + AUTH_KEY
	g.Expect(container.VolumeMounts).To(HaveLen(2)) // app-data + auth
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))

	// Remove auth
	g.Expect(Auth("server", nil)(deployment)).To(Succeed())

	// Verify unrelated resources preserved
	container = deployment.Spec.Template.Spec.Containers[0]
	g.Expect(container.Env).To(HaveLen(1))
	g.Expect(container.Env[0].Name).To(Equal("APP_ENV"))
	g.Expect(container.VolumeMounts).To(HaveLen(1))
	g.Expect(container.VolumeMounts[0].Name).To(Equal("app-data"))
	g.Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
	g.Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal("app-data"))
}
