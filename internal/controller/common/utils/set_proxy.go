package utils

import (
	"github.com/operator-framework/operator-lib/proxy"
	appsv1 "k8s.io/api/apps/v1"
)

// SetProxyEnvs set the standard environment variables for proxys "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"
func SetProxyEnvs(dep *appsv1.Deployment) {
	for i, container := range dep.Spec.Template.Spec.Containers {
		dep.Spec.Template.Spec.Containers[i].Env = append(container.Env, proxy.ReadProxyVarsFromEnv()...)
	}
}
