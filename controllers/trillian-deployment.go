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

const (
	netcat = "registry.redhat.io/rhtas-tech-preview/trillian-netcat-rhel9@sha256:b9fa895af8967cceb7a05ed7c9f2b80df047682ed11c87249ca2edba86492f6e"
)

func (r *SecuresignReconciler) ensureTrillDeployment(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, dpName string, image string, dbsecret string) (*apps.Deployment,
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
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      "trillian",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": dpName,
					"app.kubernetes.io/instance":  "trusted-artifact-signer",
					"app.kubernetes.io/name":      "trillian",
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": dpName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      "trillian",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					InitContainers: []core.Container{
						{
							Name:  "wait-for-trillian-db",
							Image: netcat,
							Command: []string{
								"sh",
								"-c",
								"until nc -z -v -w30 trillian-mysql 3306; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
							},
						},
					},
					Containers: []core.Container{
						{
							Args: []string{
								"--storage_system=mysql",
								"--quota_system=mysql",
								"--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/$(MYSQL_DATABASE)",
								"--rpc_endpoint=0.0.0.0:8091",
								"--http_endpoint=0.0.0.0:8090",
								"--alsologtostderr",
							},
							Name:  dpName,
							Image: image,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 8091,
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
									Name:  "MYSQL_HOSTNAME",
									Value: "trillian-mysql",
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
						},
					},
				},
			},
		},
	}

	// If the deployment name is trillian-logserver then add additional port 8090
	if dpName == "trillian-logserver" {
		dep.Spec.Template.Spec.Containers[0].Ports = append(dep.Spec.Template.Spec.Containers[0].Ports, core.ContainerPort{
			Protocol:      core.ProtocolTCP,
			ContainerPort: 8090,
		})
	}

	// Add --force_master=true to args for the trillian-logsigner deployment
	if dpName == "trillian-logsigner" {
		dep.Spec.Template.Spec.Containers[0].Args = append(dep.Spec.Template.Spec.Containers[0].Args, "--force_master=true")
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
