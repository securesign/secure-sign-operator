package utils

import (
	"context"
	_ "embed"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tufRepositoryPath = "/tuf-repository"
	rootKeySecretPath = "/root-key-secret"
	workdirVolumePath = "/workdir"
)

//go:embed tuf_migration_v1.sh
var script string

func EnsureTufMigrationJob(ctx context.Context, c client.Client, instance *rhtasv1.Tuf, sa string, jobLabels map[string]string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		jobSpec := &job.Spec
		jobSpec.Parallelism = ptr.To[int32](1)
		jobSpec.Completions = ptr.To[int32](1)
		jobSpec.BackoffLimit = ptr.To(int32(0))
		jobSpec.Template.Labels = jobLabels

		templateSpec := &jobSpec.Template.Spec
		templateSpec.ServiceAccountName = sa
		templateSpec.RestartPolicy = v1.RestartPolicyNever
		templateSpec.Affinity = &v1.Affinity{
			PodAffinity: &v1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels.For(constants.ComponentName, constants.DeploymentName, instance.Name),
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		}

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
		operatorName := kubernetes.FindEnvByNameOrCreate(container, "OPERATOR_NAME")
		operatorName.Value = constants.OperatorName
		tufRepo := kubernetes.FindEnvByNameOrCreate(container, "TUF_REPO")
		tufRepo.Value = tufRepositoryPath
		keydir := kubernetes.FindEnvByNameOrCreate(container, "KEYDIR")
		keydir.Value = rootKeySecretPath
		workdir := kubernetes.FindEnvByNameOrCreate(container, "WORKDIR")
		workdir.Value = workdirVolumePath

		for _, key := range instance.Spec.Keys {
			result, err := resolveServiceAddress(ctx, c, instance, key.Name)
			if err != nil {
				return err
			}
			switch key.Name {
			case rhtasv1.TufKeyRekor:
				kubernetes.FindEnvByNameOrCreate(container, "REKOR_URL").Value = result.Address
			case rhtasv1.TufKeyCTFE:
				kubernetes.FindEnvByNameOrCreate(container, "CTLOG_URL").Value = result.Address
			case rhtasv1.TufKeyFulcio:
				kubernetes.FindEnvByNameOrCreate(container, "FULCIO_URL").Value = result.Address
				kubernetes.FindEnvByNameOrCreate(container, "OIDC_ISSUERS").Value = strings.Join(result.OIDCIssuers, ",")
			case rhtasv1.TufKeyTSA:
				kubernetes.FindEnvByNameOrCreate(container, "TSA_URL").Value = result.Address
			}
		}

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
