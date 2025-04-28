package kubernetes

import (
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
		var epInterfaces []interface{}
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
