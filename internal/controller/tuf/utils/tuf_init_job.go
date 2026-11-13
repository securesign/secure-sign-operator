package utils

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/images"
	v1 "k8s.io/api/core/v1"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretsMonthPath     = "/var/run/tuf-secrets"
	targetMonthPath      = "/var/run/target"
	TufInitContainerName = "tuf-init"
)

func EnsureTufInitJob(ctx context.Context, c client.Client, instance *rhtasv1.Tuf) traits.Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
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

		container := corev1ac.Container().
			WithName(TufInitContainerName).
			WithImage(images.Registry.Get(images.Tuf)).
			WithEnv(corev1ac.EnvVar().WithName("NAMESPACE").WithValue(instance.Namespace)).
			WithCommand("/bin/bash", "-c").
			WithArgs(fmt.Sprintf("tuf-repo-init.sh %s; ", strings.Join(args, " "))+
				"exit_code=$?; "+
				"if [ $exit_code -eq 2 ]; then exit 0; else exit $exit_code; fi").
			WithVolumeMounts(
				corev1ac.VolumeMount().WithName("tuf-secrets").WithMountPath(secretsMonthPath),
				corev1ac.VolumeMount().WithName(constants.VolumeName).WithMountPath(targetMonthPath).WithReadOnly(false),
			)

		podSpec := corev1ac.PodSpec().
			WithRestartPolicy(v1.RestartPolicyNever).
			WithContainers(container).
			WithVolumes(
				corev1ac.Volume().WithName("tuf-secrets").WithProjected(secretsVolumeProjection(instance.Status.Keys)),
				corev1ac.Volume().WithName(constants.VolumeName).
					WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().WithClaimName(instance.Status.PvcName)),
			)

		job.WithSpec(batchv1ac.JobSpec().
			WithParallelism(1).
			WithCompletions(1).
			WithBackoffLimit(0).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(job.Labels).WithSpec(podSpec)))

		return nil
	}
}
