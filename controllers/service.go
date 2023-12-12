package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureService(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, name string, component string, ssapp string, port int) (*corev1.Service, error) {
	logger := log.FromContext(ctx)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": component,
				"app.kubernetes.io/name":      ssapp,
				"app.kubernetes.io/instance":  "rhtas-" + component,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/component": component,
				"app.kubernetes.io/name":      ssapp,
				"app.kubernetes.io/instance":  "rhtas-" + component,
			},
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
	err := r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, svc)
	if err != nil {
		logger.Info("Creating a new Service")
		err = r.Create(ctx, svc)
		if err != nil {
			logger.Error(err, "Failed to create new Service")
			return nil, err
		}
	}
	return svc, nil
}
