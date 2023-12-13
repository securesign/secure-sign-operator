/* # Source: trusted-artifact-signer/charts/scaffold/charts/trillian/templates/createdb/createdb-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: trusted-artifact-signer-trillian-createdb
  namespace: trillian-system
  labels:
    app.kubernetes.io/component: "mysql"
    app.kubernetes.io/name: trillian
    app.kubernetes.io/instance: trusted-artifact-signer
    helm.sh/chart: trillian-0.2.13
    app.kubernetes.io/managed-by: Helm
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      serviceAccountName: trillian-logserver
      restartPolicy: Never
      automountServiceAccountToken:
      containers:
        - name: trusted-artifact-signer-trillian-createdb
          image: "registry.redhat.io/rhtas-tech-preview/createdb-rhel9@sha256:c2067866e8cd73710bcdb218cb78bb3fcc5b314339a466de2b5af56b3b456be8"
          imagePullPolicy: "IfNotPresent"
          env:
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: MYSQL_USER
              valueFrom:
                secretKeyRef:
                  name: trillian-mysql
                  key: mysql-user
            - name: MYSQL_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: trillian-mysql
                  key: mysql-password
            - name: MYSQL_DATABASE
              valueFrom:
                secretKeyRef:
                  name: trillian-mysql
                  key: mysql-database
            - name: MYSQL_HOSTNAME
              value: trillian-mysql
            - name: MYSQL_PORT
              value: "3306"
            - name: EXIT_DIR
              value: "/var/exitdir"
          args: [
            "--db_name=$(MYSQL_DATABASE)",
            "--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/"
          ]
          volumeMounts:
            - name: exit-dir
              mountPath: "/var/exitdir"
      volumes:
        - name: exit-dir
          emptyDir: {} */

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

func (r *SecuresignReconciler) ensureCreateDbJob(ctx context.Context, m *rhtasv1alpha1.Securesign, namespace string, sA string, component string, jobName string, dbsecret string) (*batch.Job,
	error) {
	log := log.FromContext(ctx)
	imageName := "registry.redhat.io/rhtas-tech-preview/createdb-rhel9@sha256:c2067866e8cd73710bcdb218cb78bb3fcc5b314339a466de2b5af56b3b456be8"
	log.Info("ensuring job")
	// Define a new Namespace object
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "mysql",
				"app.kubernetes.io/instance":  "trusted-artifact-signer",
				"app.kubernetes.io/name":      component,
			},
		},
		Spec: batch.JobSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "mysql",
						"app.kubernetes.io/instance":  "trusted-artifact-signer",
						"app.kubernetes.io/name":      component,
					},
				},
				Spec: core.PodSpec{
					ServiceAccountName: sA,
					RestartPolicy:      core.RestartPolicyNever,
					Containers: []core.Container{
						{
							Name:  "trusted-artifact-signer-trillian-createdb",
							Image: imageName,
							Args: []string{
								"--db_name=$(MYSQL_DATABASE)",
								"--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/",
							},
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
									Name:      "exit-dir",
									MountPath: "/var/exitdir",
								},
							},
						},
					},
					Volumes: []core.Volume{
						{
							Name: "storage",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
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
