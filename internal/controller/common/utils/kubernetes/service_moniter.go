package kubernetes

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateServiceMonitor(namespace, name string, labels map[string]string, endpoints []monitoringv1.Endpoint, matchLabels map[string]string) *monitoringv1.ServiceMonitor {
	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: endpoints,
			Selector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
}

func EnsureServiceMonitorSpec(selectorLabels map[string]string, endpoints ...monitoringv1.Endpoint) func(*monitoringv1.ServiceMonitor) error {
	return func(monitor *monitoringv1.ServiceMonitor) error {
		monitor.Spec.Endpoints = endpoints
		monitor.Spec.Selector = metav1.LabelSelector{
			MatchLabels: selectorLabels,
		}
		return nil
	}
}
