package ensure

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
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

func mockSetProxyContainer() corev1.Container {
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
	return corev1.Container{Name: "test-container", Env: defaultEnv}
}

func TestSetProxyEnvs(t *testing.T) {
	type args struct {
		containers []corev1.Container
		envVars    []corev1.EnvVar
		noProxy    []string
	}
	tests := []struct {
		name   string
		args   args
		verify func(Gomega, []corev1.Container)
	}{
		{
			name: "set proxy",
			args: args{
				containers: []corev1.Container{mockSetProxyContainer()},
				envVars:    mockReadProxyVarsFromEnv(),
			},
			verify: func(g Gomega, containers []corev1.Container) {
				expectedEnvVars := append(mockReadProxyVarsFromEnv(), corev1.EnvVar{
					Name:  "answer",
					Value: "42",
				})

				g.Expect(containers).ShouldNot(BeNil())
				g.Expect(containers[0].Env).Should(HaveLen(7))
				g.Expect(containers[0].Env).Should(ConsistOf(expectedEnvVars))
			},
		},
		{
			name: "do not set proxy",
			args: args{
				containers: []corev1.Container{mockSetProxyContainer()},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				defaultEnv := mockSetProxyContainer().Env

				g.Expect(containers).ShouldNot(BeNil())
				g.Expect(containers[0].Env).Should(HaveLen(2))
				g.Expect(containers[0].Env).Should(BeEquivalentTo(defaultEnv))
			},
		},
		{
			name: "extend no_proxy",
			args: args{
				containers: []corev1.Container{mockSetProxyContainer()},
				envVars:    mockReadProxyVarsFromEnv(),
				noProxy:    []string{"extra", "socket"},
			},
			verify: func(g Gomega, containers []corev1.Container) {
				expectedEnvVars := mockReadProxyVarsFromEnv()
				for i, envVar := range expectedEnvVars {
					if strings.ToLower(envVar.Name) == "no_proxy" {
						expectedEnvVars[i].Value = "extra,socket," + envVar.Value
					}
				}

				expectedEnvVars = append(expectedEnvVars, corev1.EnvVar{
					Name:  "answer",
					Value: "42",
				})

				g.Expect(containers).ShouldNot(BeNil())
				g.Expect(containers[0].Env).Should(HaveLen(7))
				g.Expect(containers[0].Env).Should(ConsistOf(expectedEnvVars))
			},
		},
		{
			name: "do not duplicate",
			args: args{
				containers: []corev1.Container{mockSetProxyContainer()},
				envVars:    mockReadProxyVarsFromEnv(),
			},
			verify: func(g Gomega, containers []corev1.Container) {
				SetProxyEnvs(containers)

				expectedEnvVars := append(mockReadProxyVarsFromEnv(), corev1.EnvVar{
					Name:  "answer",
					Value: "42",
				})

				g.Expect(containers).ShouldNot(BeNil())
				g.Expect(containers[0].Env).Should(HaveLen(7))
				g.Expect(containers[0].Env).Should(ConsistOf(expectedEnvVars))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			for _, e := range tt.args.envVars {
				t.Setenv(e.Name, e.Value)
			}

			containers := tt.args.containers
			SetProxyEnvs(containers, tt.args.noProxy...)
			tt.verify(g, containers)
		})
	}
}
