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

func (r *SecuresignReconciler) ensureCreateCTJob(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, jobName string, fun string, trn string) (*batch.Job,
	error) {
	log := log.FromContext(ctx)
	imageName := "registry.redhat.io/rhtas-tech-preview/createctconfig-rhel9@sha256:10155f8c2b73b12599124895b2db0c9e08b2c3953df7361574fd08467c42fd04"
	log.Info("ensuring job")
	// Define a new Namespace object
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "ctlog",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "mysql",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName:           sA,
					AutomountServiceAccountToken: &[]bool{true}[0],
					RestartPolicy:                core.RestartPolicyNever,
					InitContainers: []core.Container{
						{
							Name:  "wait-for-createtree-configmap",
							Image: "registry.access.redhat.com/ubi9/ubi-minimal:latest",
							Command: []string{
								"sh",
								"-c",
								"until curl --fail --header \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt --max-time 10 https://kubernetes.default.svc/api/v1/namespaces/$(NAMESPACE)/configmaps/ctlog-config | grep '\"treeID\":'; do echo waiting for Configmap ctlog-config; sleep 5; done;",
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  "trusted-artifact-signer-ctlog-createctconfig",
							Image: imageName,
							Args: []string{
								"--configmap=ctlog-config",
								"--secret=ctlog-secret",
								"--pubkeysecret=ctlog-public-key",
								"--fulcio-url=http://fulcio-server." + fun + ".svc",
								"--trillian-server=trillian-logserver." + trn + ":8091",
								"--log-prefix=sigstorescaffolding",
							},
							Env: []core.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: namespace,
								},
							},
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
