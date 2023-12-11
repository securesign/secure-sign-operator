package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *SecuresignReconciler) ensureDeployment(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, service string, sA string, dpName string) (*apps.Deployment,
	error) {
	log := log.FromContext(ctx)
	log.Info("ensuring deployment")
	replicas := int32(1)
	// Define a new Namespace object
	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "rhats-" + m.Name,
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "rhats-" + m.Name,
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "rhats-" + m.Name,
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: "registry.redhat.io/rhel8/httpd-24@sha256:1f9e679815efdaedfe379c21f45034525228be5278ba6c71eb13f7594b694c8f",
							Ports: []core.ContainerPort{
								{
									Name:          "swarm",
									ContainerPort: 8080,
								},
							},
							Env: []core.EnvVar{
								{
									Name:  "fake",
									Value: "fake",
								},
							},
						},
					},
				},
			},
		},
	}

	// Check if this Deployment already exists else create it
	err := r.Get(ctx, client.ObjectKey{Name: dep.Name, Namespace: namespace}, dep)
	// If the Deployment doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Deployment")
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment")
			return nil, err
		}
	}
	return dep, nil
}
