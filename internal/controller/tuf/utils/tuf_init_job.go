package utils

import (
	"path/filepath"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	constants2 "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

func EnsureTufInitJob(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		// prepare args
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

		jobSpec := &job.Spec
		jobSpec.Parallelism = ptr.To[int32](1)
		jobSpec.Completions = ptr.To[int32](1)
		jobSpec.BackoffLimit = ptr.To(int32(0))
		jobSpec.Template.Labels = labels

		templateSpec := &jobSpec.Template.Spec
		templateSpec.ServiceAccountName = sa
		templateSpec.RestartPolicy = v1.RestartPolicyNever

		// initialize volumes
		secretsVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, "tuf-secrets")
		secretsVolume.VolumeSource = v1.VolumeSource{
			Projected: secretsVolumeProjection(instance.Status.Keys),
		}

		repositoryVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, constants2.VolumeName)
		repositoryVolume.VolumeSource = v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Status.PvcName,
			},
		}
		// init containers
		container := kubernetes.FindContainerByNameOrCreate(templateSpec, "tuf-init")
		container.Image = images.Registry.Get(images.Tuf)
		env := kubernetes.FindEnvByNameOrCreate(container, "NAMESPACE")
		env.Value = instance.Namespace
		container.Args = args
		container.VolumeMounts = []v1.VolumeMount{
			{
				Name:      "tuf-secrets",
				MountPath: secretsMonthPath,
			},
			{
				Name:      "repository",
				MountPath: targetMonthPath,
				ReadOnly:  false,
			},
		}

		return nil
	}
}
