package kubernetes

import (
	"github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateServiceMonitor(namespace, name string) *unstructured.Unstructured {
	sm := &unstructured.Unstructured{}
	sm.SetKind("ServiceMonitor")
	sm.SetAPIVersion("monitoring.coreos.com/v1")
	sm.SetName(name)
	sm.SetNamespace(namespace)
	return sm
}

type serviceMonitorEndpoint map[string]interface{}

func (t serviceMonitorEndpoint) toMap() map[string]interface{} {
	return t
}

func ServiceMonitorHttpsEndpoint(port, serverName string, ca *v1alpha1.SecretKeySelector) serviceMonitorEndpoint {
	result := map[string]interface{}{
		"interval": "30s",
		"port":     port,
		"scheme":   "https",
		"tlsConfig": map[string]interface{}{
			"insecureSkipVerify": true,
			"serverName":         serverName,
		},
	}
	if ca != nil {
		tlsConfig := result["tlsConfig"].(map[string]interface{})
		tlsConfig["ca"] = map[string]interface{}{
			"secret": map[string]interface{}{
				"name": ca.Name,
				"key":  ca.Key,
			},
		}
		tlsConfig["insecureSkipVerify"] = false
	}
	return result
}

func ServiceMonitorEndpoint(port string) serviceMonitorEndpoint {
	return map[string]interface{}{
		"interval": "30s",
		"port":     port,
		"scheme":   "http",
	}
}

func EnsureServiceMonitorSpec(selectorLabels map[string]string, endpoints ...serviceMonitorEndpoint) func(*unstructured.Unstructured) error {
	return func(monitor *unstructured.Unstructured) error {
		// need to convert []ServiceMonitorEndpoint to []interface{}
		epInterfaces := make([]interface{}, 0, len(endpoints))
		for _, ep := range endpoints {
			epInterfaces = append(epInterfaces, ep.toMap())
		}

		if err := unstructured.SetNestedSlice(monitor.Object, epInterfaces, "spec", "endpoints"); err != nil {
			return err
		}

		if err := unstructured.SetNestedStringMap(monitor.Object, selectorLabels, "spec", "selector", "matchLabels"); err != nil {
			return err
		}
		return nil
	}
}
