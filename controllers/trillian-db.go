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

func (r *SecuresignReconciler) ensureTrillDb(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, service string, sA string, dpName string, image string, pvcName string, dbsecret string) (*apps.Deployment,
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
				"app.kubernetes.io/name":      "rhats-mysql",
				"app.kubernetes.io/instance":  "trillian-db",
				"app.kubernetes.io/component": "mysql",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "rhats-mysql",
					"app.kubernetes.io/instance":  "trillian-db",
					"app.kubernetes.io/component": "mysql",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "rhats-mysql",
						"app.kubernetes.io/instance":  "trillian-db",
						"app.kubernetes.io/component": "mysql",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					Volumes: []core.Volume{
						{
							Name: "storage",
							VolumeSource: core.VolumeSource{
								PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: image,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 3306,
								},
							},
							// Env variables from secret trillian-mysql
							Env: []core.EnvVar{
								{
									Name:  "MYSQL_USER",
									Value: "mysql",
								},
								{
									Name: "MYSQL_PASSWORD",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-password",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name: "MYSQL_ROOT_PASSWORD",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: "mysql-root-password",
											LocalObjectReference: core.LocalObjectReference{
												Name: dbsecret,
											},
										},
									},
								},
								{
									Name:  "MYSQL_PORT",
									Value: "3306",
								},
								{
									Name:  "MYSQL_DATABASE",
									Value: "trillian",
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "storage",
									MountPath: "/var/lib/mysql",
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
