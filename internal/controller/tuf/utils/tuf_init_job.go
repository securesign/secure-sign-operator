package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/apis"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

func EnsureTufInitJob(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string, oidcIssuers []string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {
		// prepare args
		args := []string{"--operator", constants.OperatorName, "--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case rekorKey:
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
				url, err := apis.ServiceAsUrl(&instance.Spec.Rekor)
				if err != nil {
					return err
				}
				args = append(args, "--rekor-uri", url)
			case ctfeKey:
				if instance.Spec.Ctlog.Prefix == "" {
					return futils.ErrCtlogPrefixNotSpecified
				}
				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
				url, err := apis.ServiceAsUrl(&instance.Spec.Ctlog)
				if err != nil {
					return err
				}
				args = append(args, "--ctlog-uri", fmt.Sprintf("%s/%s", url, instance.Spec.Ctlog.Prefix))
			case fulcioKey:
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))
				url, err := apis.ServiceAsUrl(&instance.Spec.Fulcio)
				if err != nil {
					return err
				}
				args = append(args, "--fulcio-uri", url)
			case tsaKey:
				args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.Name))
				url, err := apis.ServiceAsUrl(&instance.Spec.Tsa)
				if err != nil {
					return err
				}
				args = append(args, "--tsa-uri", url)
			}
		}
		for _, issuer := range oidcIssuers {
			args = append(args, "--oidc-uri", issuer)
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
