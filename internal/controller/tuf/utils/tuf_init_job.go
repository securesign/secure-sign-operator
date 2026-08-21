package utils

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
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
		if instance.Spec.RootKeySecretRef == nil {
			return fmt.Errorf("rootKeySecretRef is not set")
		}

		// prepare args
		args := []string{"--operator", constants.OperatorName, "--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range trustroot.ActiveKeys(instance) {
			var resolved trustroot.Resolved
			var err error
			if key == trustroot.Fulcio {
				resolved, err = trustroot.ResolveFulcio(ctx, c, instance.Namespace, trustroot.FulcioBinding(instance))
			} else {
				resolved, err = trustroot.Resolve(ctx, c, instance.Namespace, key, trustroot.Binding(instance, key))
			}
			if err != nil {
				return fmt.Errorf("%w: %w", ErrorResolveServiceUrl, err)
			}

			switch key {
			case trustroot.Rekor:
				args = append(args, "--rekor-uri", resolved.Address)
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.String()))
			case trustroot.CTFE:
				args = append(args, "--ctlog-uri", resolved.Address)
				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.String()))
			case trustroot.Fulcio:
				args = append(args, "--fulcio-uri", resolved.Address)
				for _, issuer := range resolved.OIDCIssuers {
					args = append(args, "--oidc-uri", issuer)
				}
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.String()))
			case trustroot.TSA:
				args = append(args, "--tsa-uri", resolved.Address)
				args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.String()))
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
