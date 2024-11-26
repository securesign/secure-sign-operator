package utils

import (
	"reflect"
	"slices"

	"github.com/operator-framework/operator-lib/proxy"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

// SetProxyEnvs set the standard environment variables for proxys "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"
func SetProxyEnvs(dep *appsv1.Deployment) {
	proxyEnvs := proxy.ReadProxyVarsFromEnv()
	for i, container := range dep.Spec.Template.Spec.Containers {
		for _, e := range proxyEnvs {
			if index := slices.IndexFunc(container.Env,
				func(envVar v1.EnvVar) bool { return e.Name == envVar.Name },
			); index > -1 {
				if reflect.DeepEqual(e, container.Env[index]) {
					// variable already present
					continue
				} else {
					// overwrite
					dep.Spec.Template.Spec.Containers[i].Env[index] = e
				}
			} else {
				dep.Spec.Template.Spec.Containers[i].Env = append(dep.Spec.Template.Spec.Containers[i].Env, e)
			}
		}
	}
}
