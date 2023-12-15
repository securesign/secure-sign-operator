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

func (r *SecuresignReconciler) ensureRedisDeployment(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, dpName string) (*apps.Deployment,
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
				"app.kubernetes.io/component": "redis",
				"app.kubernetes.io/name":      "rekor",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "redis",
					"app.kubernetes.io/name":      "rekor",
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "redis",
						"app.kubernetes.io/name":      "rekor",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					Volumes: []core.Volume{
						{
							Name: "storage",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: "docker.io/redis@sha256:6c42cce2871e8dc5fb3e843ed5c4e7939d312faf5e53ff0ff4ca955a7e0b2b39",
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 6379,
								},
							},
							ReadinessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									Exec: &core.ExecAction{
										Command: []string{
											"/bin/sh",
											"-c",
											"-i",
											"test $(redis-cli -h 127.0.0.1 ping) = 'PONG'",
										},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Args: []string{
								"--bind",
								"0.0.0.0",
								"--appendonly",
								"yes",
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "storage",
									MountPath: "/data",
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
