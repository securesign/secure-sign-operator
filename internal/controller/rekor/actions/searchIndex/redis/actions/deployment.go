package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	storageVolumeName = "storage"
	configVolumeName  = "config"
	configVolumeMount = "/config"
	redisConfPath     = configVolumeMount + "/redis.conf"
	tlsInitContainer  = "enable-tls"
)

func NewDeployAction() action.Action[*rhtasv1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)
	caPath, err := tls.CAPath(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	}

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.RedisDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels),
		i.ensureRedisDeployment(instance),
		traits.WithPodSpecTraits(
			traits.TrustedCA(instance.GetTrustedCA(), actions.RedisDeploymentName),
			traits.Optional(statusTLS(instance).CertRef != nil, i.ensureTLS(statusTLS(instance), caPath)),
			traits.Proxy([]string{actions.RedisDeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), actions.RedisDeploymentName),
			traits.SecurityContexts(actions.RedisDeploymentName),
			traits.ServiceAccount(actions.RBACRedisName),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build %s deployment: %w", actions.RedisDeploymentName, err), instance,
			metav1.Condition{
				Type:               actions.RedisCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			},
		)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.RedisDeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s deployment: %w", actions.RedisDeploymentName, err), instance,
			metav1.Condition{
				Type:               actions.RedisCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			},
		)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.RedisCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Redis created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func (i deployAction) ensureRedisDeployment(instance *rhtasv1.Rekor) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		ref := instance.Status.SearchIndex.DbPasswordRef
		if ref == nil {
			return errors.New("search index db password not found")
		}
		probeCommand := []string{
			"/bin/sh",
			"-c",
			"-i",
			"test $(redis-cli -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'",
		}
		container := corev1ac.Container().
			WithName(actions.RedisDeploymentName).
			WithImage(images.Registry.Get(images.RekorRedis)).
			WithEnv(corev1ac.EnvVar().
				WithName("REDIS_PASSWORD").
				WithValueFrom(corev1ac.EnvVarSource().
					WithSecretKeyRef(corev1ac.SecretKeySelector().
						WithName(ref.Name).
						WithKey(ref.Key)))).
			WithPorts(corev1ac.ContainerPort().
				WithName("redis").
				WithContainerPort(actions.RedisDeploymentPort).
				WithProtocol(corev1.ProtocolTCP)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(storageVolumeName).
				WithMountPath("/data")).
			WithReadinessProbe(corev1ac.Probe().
				WithExec(corev1ac.ExecAction().
					WithCommand(probeCommand...)).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithLivenessProbe(corev1ac.Probe().
				WithExec(corev1ac.ExecAction().
					WithCommand(probeCommand...)).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithExec(corev1ac.ExecAction().
					WithCommand(probeCommand...)).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		podSpec := corev1ac.PodSpec().
			WithContainers(container).
			WithVolumes(corev1ac.Volume().
				WithName(storageVolumeName).
				WithEmptyDir(corev1ac.EmptyDirVolumeSource()))

		dp.
			WithSpec(appsv1ac.DeploymentSpec().
				WithReplicas(1).
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(dp.Labels).
					WithSpec(podSpec)))

		return nil
	}
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1.TLS, caPath string) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if tlsConfig.PrivateKeyRef == nil {
			return errors.New("redis TLS private key not found")
		}
		if err := traits.TLS(tlsConfig, actions.RedisDeploymentName)(podSpec); err != nil {
			return err
		}

		podSpec.WithVolumes(corev1ac.Volume().
			WithName(configVolumeName).
			WithEmptyDir(corev1ac.EmptyDirVolumeSource())).
			WithInitContainers(corev1ac.Container().
				WithName(tlsInitContainer).
				WithImage(images.Registry.Get(images.RekorRedis)).
				WithCommand("/bin/bash", "-c").
				WithArgs(fmt.Sprintf(`cp $REDIS_CONF %[1]s
echo "tls-port %[2]d" >> %[1]s
echo "port 0" >> %[1]s
echo "tls-cert-file %[3]s" >> %[1]s
echo "tls-key-file %[4]s" >> %[1]s
echo "tls-ca-cert-file %[5]s" >> %[1]s
echo "tls-auth-clients no" >> %[1]s
`, redisConfPath, actions.RedisDeploymentPort, tls.TLSCertPath, tls.TLSKeyPath, caPath)).
				WithVolumeMounts(corev1ac.VolumeMount().
					WithName(configVolumeName).
					WithMountPath(configVolumeMount)).
				WithSecurityContext(traits.SSAContainerSecurityContext()))

		container := kubernetes.FindContainerByName(podSpec, actions.RedisDeploymentName)
		container.WithVolumeMounts(corev1ac.VolumeMount().
			WithName(configVolumeName).
			WithMountPath(configVolumeMount)).
			WithEnv(corev1ac.EnvVar().
				WithName("REDIS_CONF").
				WithValue(redisConfPath))

		probeCommand := []string{
			"/bin/sh",
			"-c",
			"-i",
			fmt.Sprintf("test $(redis-cli --tls --cacert %s -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'", caPath),
		}
		container.ReadinessProbe.WithExec(corev1ac.ExecAction().
			WithCommand(probeCommand...))
		container.LivenessProbe.WithExec(corev1ac.ExecAction().
			WithCommand(probeCommand...))
		container.StartupProbe.WithExec(corev1ac.ExecAction().
			WithCommand(probeCommand...))

		return nil
	}
}
