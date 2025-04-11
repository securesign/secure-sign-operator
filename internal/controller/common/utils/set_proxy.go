package utils

import (
	"strings"

	"github.com/operator-framework/operator-lib/proxy"
	appsv1 "k8s.io/api/apps/v1"
)

// SetProxyEnvs set the standard environment variables for proxys "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"
func SetProxyEnvs(dep *appsv1.Deployment, noProxy ...string) {
	envVar := proxy.ReadProxyVarsFromEnv()
	// add extra values into no_proxy
	if len(noProxy) > 0 {
		for i, c := range envVar {
			if strings.ToLower(c.Name) == "no_proxy" {
				envVar[i].Value = strings.Join(noProxy, ",") + "," + c.Value
			}
		}
	}
	for i, container := range dep.Spec.Template.Spec.Containers {
		dep.Spec.Template.Spec.Containers[i].Env = append(container.Env, envVar...)
	}
}
