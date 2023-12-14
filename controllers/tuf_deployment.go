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

func (r *SecuresignReconciler) ensureTufDeployment(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, dpName string) (*apps.Deployment,
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
				"app.kubernetes.io/component": dpName,
				"app.kubernetes.io/name":      dpName,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": dpName,
					"app.kubernetes.io/name":      dpName,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": dpName,
						"app.kubernetes.io/name":      dpName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					Volumes: []core.Volume{
						{
							Name: "tuf-secrets",
							VolumeSource: core.VolumeSource{
								Projected: &core.ProjectedVolumeSource{
									Sources: []core.VolumeProjection{
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: "ctlog-public-key",
												},
												Items: []core.KeyToPath{
													{
														Key:  "public",
														Path: "ctfe.pub",
													},
												},
											},
										},
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: "fulcio-secret-rh",
												},
												Items: []core.KeyToPath{
													{
														Key:  "cert",
														Path: "fulcio-cert",
													},
												},
											},
										},
										{
											Secret: &core.SecretProjection{
												LocalObjectReference: core.LocalObjectReference{
													Name: "rekor-public-key",
												},
												Items: []core.KeyToPath{
													{
														Key:  "key",
														Path: "rekor-pubkey",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: "registry.redhat.io/rhtas-tech-preview/tuf-server-rhel9@sha256:413e361de99f09e617084438b2fc3c9c477f4a8e2cd65bd5f48271e66d57a9d9",
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "tuf-secets",
									MountPath: "/var/run/tuf-secrets",
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
