package utils

import (
	"path/filepath"

	"github.com/operator-framework/operator-lib/proxy"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/images"
	v1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

func CreateTufInitJob(instance *v1alpha1.Tuf, name string, sa string, labels map[string]string) *v1.Job {
	env := []core.EnvVar{
		{
			Name:  "NAMESPACE",
			Value: instance.Namespace,
		},
	}
	env = append(env, proxy.ReadProxyVarsFromEnv()...)

	args := []string{"--export-keys", instance.Spec.RootKeySecretRef.Name}
	for _, key := range instance.Spec.Keys {
		switch key.Name {
		case "rekor.pub":
			args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
		case "ctfe.pub":
			args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
		case "fulcio_v1.crt.pem":
			args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))
		case "tsa.certchain.pem":
			args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.Name))
		}
	}
	args = append(args, targetMonthPath)
	job := &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: v1.JobSpec{
			Parallelism:  ptr.To[int32](1),
			Completions:  ptr.To[int32](1),
			BackoffLimit: ptr.To(int32(0)),
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      core.RestartPolicyNever,
					Volumes: []core.Volume{
						{
							Name: "tuf-secrets",
							VolumeSource: core.VolumeSource{
								Projected: secretsVolumeProjection(instance.Status.Keys),
							},
						},
						{
							Name: "repository",
							VolumeSource: core.VolumeSource{
								PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.Status.PvcName,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  "tuf-init",
							Image: images.Registry.Get(images.Tuf),
							Env:   env,
							Args:  args,
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "tuf-secrets",
									MountPath: secretsMonthPath,
								},
								{
									Name:      "repository",
									MountPath: targetMonthPath,
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
	}
	return job
}
