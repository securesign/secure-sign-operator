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
				container.WithEnv(corev1ac.EnvVar().WithName("REKOR_URL").WithValue(resolved.Address))
			case trustroot.CTFE:
				container.WithEnv(corev1ac.EnvVar().WithName("CTLOG_URL").WithValue(resolved.Address))
			case trustroot.Fulcio:
				container.WithEnv(
					corev1ac.EnvVar().WithName("FULCIO_URL").WithValue(resolved.Address),
					corev1ac.EnvVar().WithName("OIDC_ISSUERS").WithValue(strings.Join(resolved.OIDCIssuers, ",")),
				)
			case trustroot.TSA:
				container.WithEnv(corev1ac.EnvVar().WithName("TSA_URL").WithValue(resolved.Address))
			}
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
