package utils

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

func EnsureTufInitJob(ctx context.Context, c client.Client, instance *rhtasv1.Tuf, sa string, labels map[string]string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {
		// prepare args
		args := []string{"--operator", constants.OperatorName, "--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case rhtasv1.TufKeyRekor:
				result, err := resolveServiceAddress(ctx, c, instance, key.Name)
				if err != nil {
					return err
				}
				args = append(args, "--rekor-uri", result.Address)
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
			case rhtasv1.TufKeyCTFE:
				result, err := resolveServiceAddress(ctx, c, instance, key.Name)
				if err != nil {
					return err
				}
				args = append(args, "--ctlog-uri", result.Address)
				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
			case rhtasv1.TufKeyFulcio:
				result, err := resolveServiceAddress(ctx, c, instance, key.Name)
				if err != nil {
					return err
				}
				args = append(args, "--fulcio-uri", result.Address)
				for _, issuer := range result.OIDCIssuers {
					args = append(args, "--oidc-uri", issuer)
				}
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))

			case rhtasv1.TufKeyTSA:
				result, err := resolveServiceAddress(ctx, c, instance, key.Name)
				if err != nil {
					return err
				}
				args = append(args, "--tsa-uri", result.Address)
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

		repositoryVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, constants.VolumeName)
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
		container.Command = []string{"/bin/bash", "-c"}
		container.Args = []string{
			fmt.Sprintf("tuf-repo-init.sh %s; ", strings.Join(args, " ")) +
				"exit_code=$?; " +
				"if [ $exit_code -eq 2 ]; then exit 0; else exit $exit_code; fi",
		}
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
