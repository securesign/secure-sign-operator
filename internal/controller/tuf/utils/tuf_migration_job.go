package utils

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	v1 "k8s.io/api/core/v1"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tufRepositoryPath         = "/tuf-repository"
	rootKeySecretPath         = "/root-key-secret"
	workdirVolumePath         = "/workdir"
	TufMigrationContainerName = "tuf-migration"
)

//go:embed tuf_migration_v1.sh
var script string

func EnsureTufMigrationJob(ctx context.Context, c client.Client, instance *rhtasv1.Tuf) traits.Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
		container := corev1ac.Container().
			WithName(TufMigrationContainerName).
			WithImage(images.Registry.Get(images.Tuf)).
			WithEnv(
				corev1ac.EnvVar().WithName("OPERATOR_NAME").WithValue(constants.OperatorName),
				corev1ac.EnvVar().WithName("TUF_REPO").WithValue(tufRepositoryPath),
				corev1ac.EnvVar().WithName("KEYDIR").WithValue(rootKeySecretPath),
				corev1ac.EnvVar().WithName("WORKDIR").WithValue(workdirVolumePath),
			).
			WithCommand("/bin/bash", "-c").
			WithArgs(script).
			WithVolumeMounts(
				corev1ac.VolumeMount().WithName("root-key").WithMountPath(rootKeySecretPath),
				corev1ac.VolumeMount().WithName(constants.VolumeName).WithMountPath(tufRepositoryPath).WithReadOnly(false),
				corev1ac.VolumeMount().WithName("workdir").WithMountPath(workdirVolumePath),
			)

		spec := &instance.Spec
		rekorUrl, err := serviceresolver.ResolveExternalServiceUrl(ctx, c, trustroot.FirstBinding(spec.Rekor), instance.Namespace, &rhtasv1.Rekor{})
		if err != nil {
			return fmt.Errorf("%w: %w", trustroot.ErrorResolveServiceUrl, err)
		}
		ctlogUrl, err := serviceresolver.ResolveExternalServiceUrl(ctx, c, trustroot.FirstBinding(spec.Ctlog), instance.Namespace, &rhtasv1.CTlog{})
		if err != nil {
			return fmt.Errorf("%w: %w", trustroot.ErrorResolveServiceUrl, err)
		}
		fulcioUrl, err := serviceresolver.ResolveExternalServiceUrl(ctx, c, trustroot.FirstBinding(spec.Fulcio), instance.Namespace, &rhtasv1.Fulcio{})
		if err != nil {
			return fmt.Errorf("%w: %w", trustroot.ErrorResolveServiceUrl, err)
		}
		fulcioIssuers, err := trustroot.LoadFulcioIssuers(ctx, c, instance.Namespace, spec.Fulcio, &rhtasv1.Fulcio{})
		if err != nil {
			return fmt.Errorf("%w: %w", trustroot.ErrorResolveServiceUrl, err)
		}
		container.WithEnv(
			corev1ac.EnvVar().WithName("REKOR_URL").WithValue(rekorUrl),
			corev1ac.EnvVar().WithName("CTLOG_URL").WithValue(ctlogUrl),
			corev1ac.EnvVar().WithName("FULCIO_URL").WithValue(fulcioUrl),
			corev1ac.EnvVar().WithName("OIDC_ISSUERS").WithValue(strings.Join(fulcioIssuers, ",")),
		)
		if spec.Tsa != nil {
			tsaUrl, err := serviceresolver.ResolveExternalServiceUrl(ctx, c, trustroot.FirstBinding(*spec.Tsa), instance.Namespace, &rhtasv1.TimestampAuthority{})
			if err != nil {
				return fmt.Errorf("%w: %w", trustroot.ErrorResolveServiceUrl, err)
			}
			container.WithEnv(corev1ac.EnvVar().WithName("TSA_URL").WithValue(tsaUrl))
		}

		podSpec := corev1ac.PodSpec().
			WithRestartPolicy(v1.RestartPolicyNever).
			WithContainers(container).
			WithAffinity(corev1ac.Affinity().WithPodAffinity(corev1ac.PodAffinity().
				WithRequiredDuringSchedulingIgnoredDuringExecution(corev1ac.PodAffinityTerm().
					WithLabelSelector(metav1ac.LabelSelector().WithMatchLabels(labels.For(constants.ComponentName, constants.DeploymentName, instance.Name))).
					WithTopologyKey("kubernetes.io/hostname")))).
			WithVolumes(
				corev1ac.Volume().WithName("root-key").WithSecret(corev1ac.SecretVolumeSource().WithSecretName(instance.Spec.RootKeySecretRef.Name)),
				corev1ac.Volume().WithName(constants.VolumeName).
					WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().WithClaimName(instance.Status.PvcName)),
				corev1ac.Volume().WithName("workdir").WithEmptyDir(corev1ac.EmptyDirVolumeSource()),
			)

		job.WithSpec(batchv1ac.JobSpec().
			WithParallelism(1).
			WithCompletions(1).
			WithBackoffLimit(0).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(job.Labels).WithSpec(podSpec)))

		return nil
	}
}
