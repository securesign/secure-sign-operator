package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	rekorutils "github.com/securesign/operator/internal/controller/rekor/utils"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	rekorShardingConfigVolumeName = "rekor-sharding-config"
	rekorPrivateKeyVolumeName     = "rekor-private-key-volume"
	attestationStorageVolumeName  = "storage"
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
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	labels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	trillianHost, trillianPort, err := serviceresolver.ResolveInternalGrpcService(ctx, i.Client, instance.Spec.Trillian, instance.Namespace, &rhtasv1.Trillian{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving Trillian URL: %w", err), instance, metav1.Condition{
			Type:               actions.ServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for Trillian service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}
	i.Logger.V(1).Info("trillian logserver", "address", trillianHost, "port", trillianPort)

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.ServerDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels),
		i.ensureServerDeployment(instance, trillianHost, trillianPort),
		traits.PodRequirements(instance.Spec.PodRequirements, actions.ServerDeploymentName),
		traits.WithPodSpecTraits(
			i.ensureAttestation(instance),
			traits.Proxy([]string{actions.ServerDeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), actions.ServerDeploymentName),
			traits.TrustedCA(instance.GetTrustedCA(), actions.ServerDeploymentName),
			traits.SecurityContexts(actions.ServerDeploymentName),
			traits.ServiceAccount(actions.RBACName),
			traits.Auth(instance.Spec.Auth, actions.ServerDeploymentName),
			traits.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian()),
			traits.PodExtensions(instance.Spec.PodExtensions, actions.ServerDeploymentName),
		),
	)
	if err != nil {
		if errors.Is(err, traits.ErrPodExtensionConflict) {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
		return i.Error(ctx, fmt.Errorf("could not build Rekor server deployment: %w", err), instance)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ServerDeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create server Deployment: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.ServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Deployment created",
			ObservedGeneration: instance.Generation,
		})
		i.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "DeploymentUpdated", "Updated", "Deployment updated: %s", instance.Name)
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func (i deployAction) ensureServerDeployment(instance *rhtasv1.Rekor, trillianHost, trillianPort string) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		switch {
		case instance.Status.ServerConfigRef == nil:
			return fmt.Errorf("CreateRekorDeployment: %w", rekorutils.ErrServerConfigNotSpecified)
		case instance.Status.TreeID == nil:
			return fmt.Errorf("CreateRekorDeployment: %w", rekorutils.ErrTreeNotSpecified)
		case trillianHost == "":
			return fmt.Errorf("CreateRekorDeployment: %w", rekorutils.ErrTrillianAddressNotSpecified)
		case trillianPort == "":
			return fmt.Errorf("CreateRekorDeployment: %w", rekorutils.ErrTrillianPortNotSpecified)
		}

		container := corev1ac.Container().
			WithName(actions.ServerDeploymentName).
			WithImage(images.Registry.Get(images.RekorServer)).
			WithArgs(
				"serve",
				"--trillian_log_server.address", trillianHost,
				"--trillian_log_server.port", trillianPort,
				"--trillian_log_server.sharding_config", "/sharding/sharding-config.yaml",
				"--trillian_log_server.grpc_default_service_config", `{"loadBalancingConfig":[{"round_robin":{}}]}`,
				"--rekor_server.address", "0.0.0.0",
				"--enable_retrieve_api=true",
				"--trillian_log_server.tlog_id", strconv.FormatInt(*instance.Status.TreeID, 10),
				"--log_type", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod)),
			).
			WithPorts(corev1ac.ContainerPort().
				WithName("rekor-server").
				WithContainerPort(3000).
				WithProtocol(corev1.ProtocolTCP)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(rekorShardingConfigVolumeName).
				WithMountPath("/sharding")).
			WithLivenessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/ping").
					WithPort(intstr.FromInt32(3000))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/api/v1/log").
					WithPort(intstr.FromInt32(3000))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(5).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/api/v1/log").
					WithPort(intstr.FromInt32(3000))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		podSpec := corev1ac.PodSpec().
			WithVolumes(corev1ac.Volume().
				WithName(rekorShardingConfigVolumeName).
				WithConfigMap(corev1ac.ConfigMapVolumeSource().
					WithName(instance.Status.ServerConfigRef.Name)))

		if fips.Enabled() {
			container.WithArgs("--client-signing-algorithms", fips.ClientSigningAlgorithms)
		}
		if instance.Spec.MaxRequestBodySize != nil {
			container.WithArgs("--max_request_body_size", strconv.FormatInt(*instance.Spec.MaxRequestBodySize, 10))
		}

		if err := i.ensureSigner(instance, podSpec, container); err != nil {
			return err
		}

		if err := i.ensureSearchIndex(instance, container); err != nil {
			return err
		}
		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			container.WithPorts(corev1ac.ContainerPort().
				WithName("monitoring").
				WithContainerPort(2112).
				WithProtocol(corev1.ProtocolTCP))
		}
		podSpec.WithContainers(container)

		dp.WithSpec(appsv1ac.DeploymentSpec().
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
			WithStrategy(appsv1ac.DeploymentStrategy().WithType(appsv1.RecreateDeploymentStrategyType)).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(dp.Labels).
				WithSpec(podSpec)))
		return nil
	}
}

func (i deployAction) ensureSigner(instance *rhtasv1.Rekor, podSpec *corev1ac.PodSpecApplyConfiguration, container *corev1ac.ContainerApplyConfiguration) error {
	switch instance.Spec.Signer.Type {
	case rhtasv1.SignerTypeKMS:
		if instance.Spec.Signer.Kms == nil {
			return fmt.Errorf("kms config is required when type is %q", rhtasv1.SignerTypeKMS)
		}
		container.WithArgs("--rekor_server.signer", instance.Spec.Signer.Kms.KeyResource)
	case rhtasv1.SignerTypeMemory:
		container.WithArgs("--rekor_server.signer", "memory")
	default:
		if instance.Status.Signer.KeyRef == nil {
			return rekorutils.ErrSignerKeyNotSpecified
		}
		container.WithArgs("--rekor_server.signer", "/key/private")
		container.WithVolumeMounts(corev1ac.VolumeMount().
			WithName(rekorPrivateKeyVolumeName).
			WithMountPath("/key").
			WithReadOnly(true))

		podSpec.WithVolumes(corev1ac.Volume().
			WithName(rekorPrivateKeyVolumeName).
			WithSecret(corev1ac.SecretVolumeSource().
				WithSecretName(instance.Status.Signer.KeyRef.Name).
				WithItems(corev1ac.KeyToPath().
					WithKey(instance.Status.Signer.KeyRef.Key).
					WithPath(constants.KeyPrivate))))

		if ref := instance.Status.Signer.PasswordRef; ref != nil {
			container.WithArgs("--rekor_server.signer-passwd", "$(SIGNER_PASSWORD)")
			container.WithEnv(corev1ac.EnvVar().
				WithName("SIGNER_PASSWORD").
				WithValueFrom(corev1ac.EnvVarSource().
					WithSecretKeyRef(corev1ac.SecretKeySelector().WithName(ref.Name).WithKey(ref.Key))))
		}
	}
	return nil
}

func (i deployAction) ensureSearchIndex(instance *rhtasv1.Rekor, container *corev1ac.ContainerApplyConfiguration) error {
	options, err := searchIndex.Resolve(instance)
	if err != nil {
		return err
	}
	if ref := options.PasswordRef; ref != nil {
		container.WithEnv(corev1ac.EnvVar().
			WithName(searchIndex.RedisPasswordEnv).
			WithValueFrom(corev1ac.EnvVarSource().
				WithSecretKeyRef(corev1ac.SecretKeySelector().WithName(ref.Name).WithKey(ref.Key))))
	}
	if options.Redis != nil {
		container.WithArgs(ensureRedisParams(options.Redis)...)
	} else {
		container.WithArgs(ensureMysqlParams(options.MySQLDSN)...)
	}
	return nil
}

func (i deployAction) ensureTlsTrillian() traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		kubernetes.FindContainerByName(podSpec, actions.ServerDeploymentName).WithArgs("--trillian_log_server.tls=true")
		return nil
	}
}

func (i deployAction) ensureAttestation(instance *rhtasv1.Rekor) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		container := kubernetes.FindContainerByName(podSpec, actions.ServerDeploymentName)
		bucketURL := instance.Spec.Attestations.URL
		if bucketURL == "" {
			bucketURL = "file:///var/run/attestations?no_tmp_dir=true"
		}
		container.WithArgs(fmt.Sprintf("--enable_attestation_storage=%t", utils.IsEnabled(instance.Spec.Attestations.Enabled)),
			"--attestation_storage_bucket", bucketURL,
		)
		if instance.Spec.Attestations.MaxSize != nil {
			maxSize, ok := instance.Spec.Attestations.MaxSize.AsInt64()
			if !ok {
				return errors.New("attestation max size must be an integer")
			}
			container.WithArgs("--max_attestation_size", strconv.FormatInt(maxSize, 10))
		}

		if enabledFileAttestationStorage(instance) {
			container.WithVolumeMounts(corev1ac.VolumeMount().WithName(attestationStorageVolumeName).WithMountPath("/var/run/attestations"))
		}

		if enabledFileAttestationStorage(instance) {
			podSpec.WithVolumes(corev1ac.Volume().WithName(attestationStorageVolumeName).
				WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().WithClaimName(instance.Status.PvcName)))
		}
		return nil
	}
}

func ensureRedisParams(options *redis.RedisOptions) []string {
	args := []string{
		"--search_index.storage_provider", "redis",
		"--redis_server.address", options.Host,
	}
	if options.Port != "" {
		args = append(args, "--redis_server.port", options.Port)
	}
	if options.Password != "" {
		args = append(args, "--redis_server.password", options.Password)
	}
	if options.TlsEnabled {
		args = append(args, "--redis_server.enable-tls=true")
	}
	return args
}

func ensureMysqlParams(url string) []string {
	return []string{
		"--search_index.storage_provider", "mysql",
		"--search_index.mysql.dsn", url,
		"--search_index.mysql.max_open_connections", "30",
		"--search_index.mysql.max_idle_connections", "10",
		"--search_index.mysql.conn_max_lifetime", "10m",
		"--search_index.mysql.conn_max_idletime", "2m",
	}
}
