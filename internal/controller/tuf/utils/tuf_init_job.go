package utils

import (
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func CreateTufInitJob(instance *v1alpha1.Tuf, name string, sa string, labels map[string]string) *v1.Job {
	env := []core.EnvVar{
		{
			Name:  "NAMESPACE",
			Value: instance.Namespace,
		},
	}
	env = append(env, proxy.ReadProxyVarsFromEnv()...)
	job := &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: v1.JobSpec{
			Parallelism: ptr.To[int32](1),
			Completions: ptr.To[int32](1),
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
							Image: constants.TufImage,
							Env:   env,
							Args: []string{
								"-mode", "init-no-overwrite",
								"-target-dir", "/var/run/target",
								"-keyssecret", instance.Spec.RootKeySecretRef.Name,
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "tuf-secrets",
									MountPath: "/var/run/tuf-secrets",
								},
								{
									Name:      "repository",
									MountPath: "/var/run/target",
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
