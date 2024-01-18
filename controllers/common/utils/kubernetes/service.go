package kubernetes

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateService(namespace string, name string, port int, labels map[string]string) *corev1.Service {
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
					Name:       name,
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
				},
			},
		},
	}
}

func SearchForInternalUrl(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (string, error) {
	list := &corev1.ServiceList{}
	err := cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels), client.Limit(1))
	if err != nil {
		return "", err
	}
	if len(list.Items) != 1 {
		return "", errors.New("component not found")
	}
	return fmt.Sprintf("%s.%s.svc.cluster.local", list.Items[0].Name, list.Items[0].Namespace), nil

}
