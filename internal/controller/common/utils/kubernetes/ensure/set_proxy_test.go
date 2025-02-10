package ensure

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Mock function to simulate proxy.ReadProxyVarsFromEnv
func mockReadProxyVarsFromEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "HTTP_PROXY", Value: "http://proxy.example.com"},
		{Name: "http_proxy", Value: "http://proxy.example.com"},
		{Name: "HTTPS_PROXY", Value: "https://proxy.example.com"},
		{Name: "https_proxy", Value: "https://proxy.example.com"},
		{Name: "NO_PROXY", Value: "localhost,127.0.0.1"},
		{Name: "no_proxy", Value: "localhost,127.0.0.1"},
	}
}

func TestSetProxyEnvs(t *testing.T) {
	g := NewWithT(t)
	defaultEnv := []corev1.EnvVar{
		{
			Name:  "answer",
			Value: "42",
		},
		{
			Name:  "no_proxy",
			Value: "toBeOverwritten",
		},
	}

	// Define a mock deployment
	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
							Env:  defaultEnv,
						},
					},
				},
			},
		},
	}

	SetProxyEnvs(dep.Spec.Template.Spec.Containers)

	g.Expect(dep.Spec.Template.Spec.Containers).ShouldNot(BeNil())
	g.Expect(dep.Spec.Template.Spec.Containers[0].Env).Should(HaveLen(2))
	g.Expect(dep.Spec.Template.Spec.Containers[0].Env).Should(BeEquivalentTo(defaultEnv))

	for _, e := range mockReadProxyVarsFromEnv() {
		t.Setenv(e.Name, e.Value)
	}

	SetProxyEnvs(dep.Spec.Template.Spec.Containers)

	expectedEnvVars := append(mockReadProxyVarsFromEnv(), corev1.EnvVar{
		Name:  "answer",
		Value: "42",
	})

	g.Expect(dep.Spec.Template.Spec.Containers).ShouldNot(BeNil())
	g.Expect(dep.Spec.Template.Spec.Containers[0].Env).Should(HaveLen(7))
	g.Expect(dep.Spec.Template.Spec.Containers[0].Env).Should(ConsistOf(expectedEnvVars))

	// ensure no duplicates
	SetProxyEnvs(dep.Spec.Template.Spec.Containers)
	g.Expect(dep.Spec.Template.Spec.Containers).ShouldNot(BeNil())
	g.Expect(dep.Spec.Template.Spec.Containers[0].Env).Should(HaveLen(7))
}
