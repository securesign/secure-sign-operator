package utils

import (
	_ "embed"
	"fmt"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/apis"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	tsaActions "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	tufRepositoryPath = "/tuf-repository"
	rootKeySecretPath = "/root-key-secret"
	workdirVolumePath = "/workdir"
)

//go:embed fix_logId.sh
var script string

func EnsureTufMigrationJob(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string, oidcIssuers []string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		jobSpec := &job.Spec
		jobSpec.Parallelism = ptr.To[int32](1)
		jobSpec.Completions = ptr.To[int32](1)
		jobSpec.BackoffLimit = ptr.To(int32(0))
		jobSpec.Template.Labels = labels

		templateSpec := &jobSpec.Template.Spec
		templateSpec.ServiceAccountName = sa
		templateSpec.RestartPolicy = v1.RestartPolicyNever

		rootKeyVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, "root-key")
		rootKeyVolume.VolumeSource = v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: instance.Spec.RootKeySecretRef.Name,
			},
		}
		repositoryVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, constants.VolumeName)
		repositoryVolume.VolumeSource = v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Status.PvcName,
			},
		}
		workdirVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, "workdir")
		workdirVolume.VolumeSource = v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		}

		// extract cosign binary from client-server image
		cosignContainer := kubernetes.FindInitContainerByNameOrCreate(templateSpec, "extract-cosign")
		cosignContainer.Image = images.Registry.Get(images.ClientServer)
		cosignContainer.Command = []string{"/bin/sh", "-c"}
		cosignContainer.Args = []string{
			"cp /var/www/html/clients/linux/cosign-amd64.gz /workdir/cosign.gz",
		}
		cosignContainer.VolumeMounts = []v1.VolumeMount{
			{
				Name:      "workdir",
				MountPath: workdirVolumePath,
			},
		}

		container := kubernetes.FindContainerByNameOrCreate(templateSpec, "tuf-migration")
		// tuf image is ubi-based so it has tooling installed
		container.Image = images.Registry.Get(images.Tuf)
		env := kubernetes.FindEnvByNameOrCreate(container, "TUF_REPO")
		env.Value = tufRepositoryPath
		env = kubernetes.FindEnvByNameOrCreate(container, "KEYDIR")
		env.Value = rootKeySecretPath
		env = kubernetes.FindEnvByNameOrCreate(container, "WORKDIR")
		env.Value = workdirVolumePath

		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case rekorKey:
				env = kubernetes.FindEnvByNameOrCreate(container, "REKOR_URL")
				url, err := apis.ServiceAsUrl(&instance.Spec.Rekor)
				if err != nil {
					return err
				}
				env.Value = url
			case ctfeKey:
				if instance.Spec.Ctlog.Prefix == "" {
					return futils.ErrCtlogPrefixNotSpecified
				}
				env = kubernetes.FindEnvByNameOrCreate(container, "CTLOG_URL")
				url, err := apis.ServiceAsUrl(&instance.Spec.Ctlog)
				if err != nil {
					return err
				}
				env.Value = fmt.Sprintf("%s/%s", url, instance.Spec.Ctlog.Prefix)
			case fulcioKey:
				env = kubernetes.FindEnvByNameOrCreate(container, "FULCIO_URL")
				url, err := apis.ServiceAsUrl(&instance.Spec.Fulcio)
				if err != nil {
					return err
				}
				env.Value = url
			case tsaKey:
				env = kubernetes.FindEnvByNameOrCreate(container, "TSA_URL")
				url, err := apis.ServiceAsUrl(&instance.Spec.Tsa)
				if err != nil {
					return err
				}
				env.Value = fmt.Sprintf("%s%s", url, tsaActions.TimestampPath)
			}
		}

		env = kubernetes.FindEnvByNameOrCreate(container, "OIDC_ISSUERS")
		env.Value = strings.Join(oidcIssuers, ",")

		container.Command = []string{"/bin/bash", "-c"}
		container.Args = []string{script}

		container.VolumeMounts = []v1.VolumeMount{
			{
				Name:      "root-key",
				MountPath: rootKeySecretPath,
			},
			{
				Name:      constants.VolumeName,
				MountPath: tufRepositoryPath,
				ReadOnly:  false,
			},
			{
				Name:      "workdir",
				MountPath: workdirVolumePath,
			},
		}

		return nil
	}
}
