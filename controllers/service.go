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

	// if trillian-logsigner add an additional service port of 8090
	if component == "trillian-logserver" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "8090-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       8090,
			TargetPort: intstr.FromInt(8090),
		})
	}
	// if rekor-server add an additional service port of 2112
	if component == "rekor-server" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "3000-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       80,
			TargetPort: intstr.FromInt(3000),
		})
	}
	// if fulcio-system add an additional service port of 5554 2112
	if component == "fulcio-server" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "5554-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       5554,
			TargetPort: intstr.FromInt(5554),
		})
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "80-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       80,
			TargetPort: intstr.FromInt(5555),
		})
	}
	// if ctlog add an additional service port of 80
	if component == "ctlog" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "80-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       80,
			TargetPort: intstr.FromInt(6962),
		})
	}
	// if tuf-server replace targetPort with 8080 instead of 80
	if component == "tuf-server" {
		svc.Spec.Ports[0].TargetPort = intstr.FromInt(8080)
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
