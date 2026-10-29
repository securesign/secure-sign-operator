package monitor

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// clearProxyEnv unsets proxy env vars for the test so ambient settings can't leak into assertions.
func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		prev, ok := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("failed to unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(name, prev)
			}
		})
	}
}

// buildMonitorStatefulSet applies the real mutators in the same order as Handle's CreateOrUpdate.
func buildMonitorStatefulSet(t *testing.T) *v1.StatefulSet {
	t.Helper()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ctlog",
			Namespace: "default",
		},
	}

	ss := &v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      actions.MonitorStatefulSetName,
			Namespace: instance.Namespace,
		},
	}

	action := statefulSetAction{}
	ensures := []func(*v1.StatefulSet) error{
		action.ensureMonitorStatefulSet(instance, actions.RBACMonitorName, map[string]string{"app": "ctlog-monitor"}, "https://ctlog.example.com", "https://tuf.example.com"),
		action.ensureInitContainer("https://ctlog.example.com", "https://tuf.example.com"),
		func(object *v1.StatefulSet) error {
			return ensure.Proxy()(&object.Spec.Template.Spec)
		},
	}
	for _, en := range ensures {
		if err := en(ss); err != nil {
			t.Fatalf("unexpected error applying mutator: %v", err)
		}
	}
	return ss
}

func findEnv(envs []core.EnvVar, name string) *core.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func TestEnsureProxyPropagatesToContainerAndInitContainer(t *testing.T) {
	g := NewWithT(t)

	t.Setenv("HTTP_PROXY", "http://proxy.example.com:3128")
	t.Setenv("HTTPS_PROXY", "https://proxy.example.com:3128")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")

	ss := buildMonitorStatefulSet(t)

	g.Expect(ss.Spec.Template.Spec.Containers).Should(HaveLen(1))
	g.Expect(ss.Spec.Template.Spec.InitContainers).Should(HaveLen(1))

	container := ss.Spec.Template.Spec.Containers[0]
	g.Expect(container.Name).Should(Equal(actions.MonitorStatefulSetName))
	httpProxy := findEnv(container.Env, "HTTP_PROXY")
	g.Expect(httpProxy).ShouldNot(BeNil(), "HTTP_PROXY should be set on the main container")
	g.Expect(httpProxy.Value).Should(Equal("http://proxy.example.com:3128"))

	httpsProxy := findEnv(container.Env, "HTTPS_PROXY")
	g.Expect(httpsProxy).ShouldNot(BeNil(), "HTTPS_PROXY should be set on the main container")
	g.Expect(httpsProxy.Value).Should(Equal("https://proxy.example.com:3128"))

	noProxy := findEnv(container.Env, "NO_PROXY")
	g.Expect(noProxy).ShouldNot(BeNil(), "NO_PROXY should be set on the main container")
	g.Expect(noProxy.Value).Should(Equal("localhost,127.0.0.1"))

	initContainer := ss.Spec.Template.Spec.InitContainers[0]
	g.Expect(initContainer.Name).Should(Equal("tuf-init"))
	initHTTPProxy := findEnv(initContainer.Env, "HTTP_PROXY")
	g.Expect(initHTTPProxy).ShouldNot(BeNil(), "HTTP_PROXY should be set on the tuf-init init container")
	g.Expect(initHTTPProxy.Value).Should(Equal("http://proxy.example.com:3128"))

	initHTTPSProxy := findEnv(initContainer.Env, "HTTPS_PROXY")
	g.Expect(initHTTPSProxy).ShouldNot(BeNil(), "HTTPS_PROXY should be set on the tuf-init init container")
	g.Expect(initHTTPSProxy.Value).Should(Equal("https://proxy.example.com:3128"))

	initNoProxy := findEnv(initContainer.Env, "NO_PROXY")
	g.Expect(initNoProxy).ShouldNot(BeNil(), "NO_PROXY should be set on the tuf-init init container")
	g.Expect(initNoProxy.Value).Should(Equal("localhost,127.0.0.1"))
}

func TestEnsureProxyAddsNoEnvVarsWhenUnconfigured(t *testing.T) {
	g := NewWithT(t)

	clearProxyEnv(t)

	ss := buildMonitorStatefulSet(t)

	g.Expect(ss.Spec.Template.Spec.Containers).Should(HaveLen(1))
	g.Expect(ss.Spec.Template.Spec.InitContainers).Should(HaveLen(1))

	assertNoProxyEnv(g, ss.Spec.Template.Spec.Containers[0].Env, "main container")
	assertNoProxyEnv(g, ss.Spec.Template.Spec.InitContainers[0].Env, "tuf-init init container")
}

func assertNoProxyEnv(g Gomega, envs []core.EnvVar, containerDesc string) {
	for _, e := range envs {
		g.Expect(strings.ToLower(e.Name)).ShouldNot(ContainSubstring("proxy"), "%s should not have a %q env var when no proxy is configured", containerDesc, e.Name)
	}
}
