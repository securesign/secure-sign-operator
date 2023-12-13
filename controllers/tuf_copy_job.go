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

func (r *SecuresignReconciler) ensureTufCopyJob(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, jobName string, rkn string, fun string, ctn string) (*batch.Job,
	error) {
	log := log.FromContext(ctx)
	imageName := "registry.redhat.io/openshift4/ose-cli:latest"
	log.Info("ensuring job", jobName, "in namespace", namespace)
	// Define a new Namespace object
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": jobName,
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      "tuf-secret-copy",
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": jobName,
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      "tuf-secret-copy",
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					RestartPolicy:      core.RestartPolicyOnFailure,
					InitContainers: []core.Container{
						{
							Name:    "wait-for-rekor-deployment-readiness",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"kubectl rollout status deployment rekor-system --timeout=120s -n " + rkn,
							},
						},
						{
							Name:    "wait-for-fulcio-deployment-readiness",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"kubectl rollout status deployment fulcio-server --timeout=120s -n " + fun,
							},
						},
						{
							Name:    "wait-for-ctlog-deployment-readiness",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"kubectl rollout status deployment ctlog --timeout=120s -n " + ctn,
							},
						},
					},
					Containers: []core.Container{
						{
							Name:    "copy-rekor-secret",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"curl rekor-server." + rkn + ".svc.cluster.local/api/v1/log/publicKey -o /tmp/key -v && kubectl create secret generic rekor-public-key --from-file=key=/tmp/key -n" + namespace,
							},
						},
						{
							Name:    "copy-fulcio-secret",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"kubectl -n " + fun + " get secrets fulcio-secret-rh -oyaml | sed 's/namespace: .*/namespace: " + namespace + "/' | kubectl apply -f -",
							},
						},
						{
							Name:    "copy-ctlog-secret",
							Image:   imageName,
							Command: []string{"/bin/sh"},
							Args: []string{
								"-c",
								"kubectl -n " + ctn + " get secrets ctlog-public-key -oyaml | sed 's/namespace: .*/namespace: " + namespace + "/' | kubectl apply -f -",
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
