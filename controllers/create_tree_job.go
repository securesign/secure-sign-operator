package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *SecuresignReconciler) ensureCTRekorJob(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, component string, jobName string, trn string) (*batch.Job,
	error) {
	log := log.FromContext(ctx)
	imageName := "registry.redhat.io/rhtas-tech-preview/createtree-rhel9@sha256:8a80def74e850f2b4c73690f86669a1fe52c1043c175610750abb4644e63d4ab"
	log.Info("ensuring job")
	// Define a new Namespace object
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "server",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      component,
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "server",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      component,
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					RestartPolicy:      core.RestartPolicyNever,
					Containers: []core.Container{
						{
							Name:  "trusted-artifact-signer-rekor-createtree",
							Image: imageName,
							Args: []string{
								"--namespace=$(NAMESPACE)",
								"--configmap=rekor-config",
								"--display_name=rekortree",
								"--admin_server=trillian-logserver." + trn + ":8091",
								"--force=false",
							},
							Env: []core.EnvVar{
								{
									Name: "NAMESPACE",
									ValueFrom: &core.EnvVarSource{
										FieldRef: &core.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							Resources: core.ResourceRequirements{},
						},
					},
				},
			},
		},
	}

	// Check if this Job already exists else create it
	err := r.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: namespace}, job)
	// If the Job doesn't exist, create it but if it does, do nothing
	if err != nil {
		log.Info("Creating a new Job")
		err = r.Create(ctx, job)
		if err != nil {
			log.Error(err, "Failed to create new Job")
			return nil, err
		}
	}
	return job, nil
}
