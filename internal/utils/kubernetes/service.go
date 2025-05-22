package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateService(namespace string, name string, portName string, port int, targetPort int32, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       portName,
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(port),
					TargetPort: intstr.FromInt32(targetPort),
				},
			},
		},
	}
}

func EnsureServiceSpec(selectorLabels map[string]string, ports ...corev1.ServicePort) func(*corev1.Service) error {
	return func(svc *corev1.Service) error {
		spec := &svc.Spec
		spec.Selector = selectorLabels
		spec.Ports = ports
		return nil
	}
}
