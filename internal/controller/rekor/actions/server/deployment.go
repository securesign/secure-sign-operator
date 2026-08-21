package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"

	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	rekorutils "github.com/securesign/operator/internal/controller/rekor/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	var (
		err     error
		changed bool
	)
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

	if changed, err = kubernetes.ApplySSA(ctx, i.Client,
		&v2.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ServerDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureServerDeployment(instance, actions.RBACName, labels, trillianHost, trillianPort),
		deployment.PodRequirements(instance.Spec.PodRequirements, actions.ServerDeploymentName),
		deployment.PodSecurityContext(),
		i.ensureAttestation(instance),
		ensure.ControllerReference[*v2.Deployment](instance, i.Client),
		ensure.Labels[*v2.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Auth(actions.ServerDeploymentName, instance.Spec.Auth),
		deployment.Proxy(),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.ServerDeploymentName),
		ensure.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian()),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}

	if !changed {
		return i.Continue()
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  state.Creating.String(),
		Message: "Deployment created",
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func (i deployAction) ensureServerDeployment(instance *rhtasv1.Rekor, sa string, labels map[string]string, trillianHost, trillianPort string) func(*v2.Deployment) error {
	return func(dp *v2.Deployment) error {
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

		spec := &dp.Spec
		spec.Strategy = v2.DeploymentStrategy{
			Type: "Recreate",
		}
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.ServerDeploymentName)
		container.Image = images.Registry.Get(images.RekorServer)

		args := []string{
			"serve",
			"--trillian_log_server.address", trillianHost,
			"--trillian_log_server.port", trillianPort,
			"--trillian_log_server.sharding_config", "/sharding/sharding-config.yaml",
			"--trillian_log_server.grpc_default_service_config", `{"loadBalancingConfig":[{"round_robin":{}}]}`,

			"--rekor_server.address", "0.0.0.0",
			// boolean flag MUST be without parameter (default value) or use the equal sign (https://github.com/spf13/pflag?tab=readme-ov-file#command-line-flag-syntax)
			"--enable_retrieve_api=true",
			"--trillian_log_server.tlog_id", strconv.FormatInt(*instance.Status.TreeID, 10),
			"--log_type", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod)),
		}

		const privateKeyVolumeName = "rekor-private-key-volume"

		switch instance.Spec.Signer.Type {
		case rhtasv1.RekorSignerTypeKMS:
			if instance.Spec.Signer.Kms == nil {
				return fmt.Errorf("kms config is required when type is %q", rhtasv1.RekorSignerTypeKMS)
			}
			args = append(args, "--rekor_server.signer", instance.Spec.Signer.Kms.KeyResource)
		case rhtasv1.RekorSignerTypeMemory:
			args = append(args, "--rekor_server.signer", "memory")
		default:
			// file signer: args and resources handled by OptionalToggle below
		}

		isFileSigner := instance.Spec.Signer.Type != rhtasv1.RekorSignerTypeKMS && instance.Spec.Signer.Type != rhtasv1.RekorSignerTypeMemory
		if err := ensure.OptionalToggle(isFileSigner, ensure.Toggleable[*v1.PodSpec]{
			Ensure: func(spec *v1.PodSpec) error {
				if instance.Status.Signer.KeyRef == nil {
					return rekorutils.ErrSignerKeyNotSpecified
				}
				args = append(args, "--rekor_server.signer", "/key/private")

				privateVolume := kubernetes.FindVolumeByNameOrCreate(spec, privateKeyVolumeName)
				if privateVolume.Secret == nil {
					privateVolume.Secret = &v1.SecretVolumeSource{}
				}
				privateVolume.Secret.SecretName = instance.Status.Signer.KeyRef.Name
				privateVolume.Secret.Items = []v1.KeyToPath{
					{
						Key:  instance.Status.Signer.KeyRef.Key,
						Path: constants.KeyPrivate,
					},
				}

				c := kubernetes.FindContainerByNameOrCreate(spec, actions.ServerDeploymentName)
				volumeMount := kubernetes.FindVolumeMountByNameOrCreate(c, privateKeyVolumeName)
				volumeMount.MountPath = "/key"
				volumeMount.ReadOnly = true

				if instance.Status.Signer.PasswordRef != nil {
					args = append(args, "--rekor_server.signer-passwd", "$(SIGNER_PASSWORD)")
					env := kubernetes.FindEnvByNameOrCreate(c, "SIGNER_PASSWORD")
					env.ValueFrom = &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							Key: instance.Status.Signer.PasswordRef.Key,
							LocalObjectReference: v1.LocalObjectReference{
								Name: instance.Status.Signer.PasswordRef.Name,
							},
						},
					}
				}
				return nil
			},
			Managed: &v1.PodSpec{
				Volumes: []v1.Volume{{Name: privateKeyVolumeName}},
				Containers: []v1.Container{{
					Name:         actions.ServerDeploymentName,
					VolumeMounts: []v1.VolumeMount{{Name: privateKeyVolumeName}},
					Env:          []v1.EnvVar{{Name: "SIGNER_PASSWORD"}},
				}},
			},
		})(&template.Spec); err != nil {
			return err
		}

		if fips.Enabled() {
			args = append(args, "--client-signing-algorithms", fips.ClientSigningAlgorithms)
		}

		if instance.Spec.MaxRequestBodySize != nil {
			args = append(args, "--max_request_body_size", fmt.Sprintf("%d", *instance.Spec.MaxRequestBodySize))
		}
		container.Args = args
		if err := searchIndex.EnsureSearchIndex(instance, ensureRedisParams(), ensureMysqlParams())(container); err != nil {
			return err
		}

		serverPort := kubernetes.FindPortByNameOrCreate(container, "rekor-server")
		serverPort.ContainerPort = 3000

		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			monitoringPort := kubernetes.FindPortByNameOrCreate(container, "monitoring")
			monitoringPort.ContainerPort = 2112
			monitoringPort.Protocol = v1.ProtocolTCP
		}

		var shardingVolumeName = "rekor-sharding-config"
		shardingVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, shardingVolumeName)
		if shardingVolume.ConfigMap == nil {
			shardingVolume.ConfigMap = &v1.ConfigMapVolumeSource{}
		}
		shardingVolume.ConfigMap.Name = instance.Status.ServerConfigRef.Name

		shardingVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, shardingVolumeName)
		shardingVolumeMount.MountPath = "/sharding"

		if container.LivenessProbe == nil {
			container.LivenessProbe = &v1.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &v1.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/ping"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.LivenessProbe.InitialDelaySeconds = 0
		container.LivenessProbe.PeriodSeconds = 10
		container.LivenessProbe.TimeoutSeconds = 1
		container.LivenessProbe.FailureThreshold = 3

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &v1.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &v1.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/api/v1/log"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.ReadinessProbe.InitialDelaySeconds = 0
		container.ReadinessProbe.PeriodSeconds = 10
		container.ReadinessProbe.TimeoutSeconds = 5
		container.ReadinessProbe.FailureThreshold = 3

		if container.StartupProbe == nil {
			container.StartupProbe = &v1.Probe{}
		}
		if container.StartupProbe.HTTPGet == nil {
			container.StartupProbe.HTTPGet = &v1.HTTPGetAction{}
		}
		container.StartupProbe.HTTPGet.Path = "/api/v1/log"
		container.StartupProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.StartupProbe.PeriodSeconds = 5
		container.StartupProbe.TimeoutSeconds = 5
		container.StartupProbe.FailureThreshold = 12

		return nil
	}
}

func (i deployAction) ensureTlsTrillian() func(*v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ServerDeploymentName)
		// boolean flag MUST be without parameter (default value) or use the equal sign (https://github.com/spf13/pflag?tab=readme-ov-file#command-line-flag-syntax)
		container.Args = append(container.Args, "--trillian_log_server.tls=true")
		return nil
	}
}

func (i deployAction) ensureAttestation(instance *rhtasv1.Rekor) func(*v2.Deployment) error {
	const storageVolumeName = "storage"
	return func(dp *v2.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ServerDeploymentName)
		enabled := utils.IsEnabled(instance.Spec.Attestations.Enabled)

		// boolean flag MUST be without parameter (default value) or use the equal sign (https://github.com/spf13/pflag?tab=readme-ov-file#command-line-flag-syntax)
		container.Args = append(container.Args, fmt.Sprintf("--enable_attestation_storage=%t", enabled))

		bucketUrl := instance.Spec.Attestations.Url
		if bucketUrl == "" {
			bucketUrl = "file:///var/run/attestations?no_tmp_dir=true"
		}
		container.Args = append(container.Args, "--attestation_storage_bucket", bucketUrl)

		if instance.Spec.Attestations.MaxSize != nil {
			maxSize, ok := instance.Spec.Attestations.MaxSize.AsInt64()
			if !ok {
				return errors.New("attestation max size must be an integer")
			}
			container.Args = append(container.Args, "--max_attestation_size", strconv.FormatInt(maxSize, 10))
		}

		// File storage
		if err := ensure.OptionalToggle(enabledFileAttestationStorage(instance), ensure.Toggleable[*v1.PodSpec]{
			Ensure: func(spec *v1.PodSpec) error {
				storageVolume := kubernetes.FindVolumeByNameOrCreate(spec, storageVolumeName)
				if storageVolume.PersistentVolumeClaim == nil {
					storageVolume.PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{}
				}
				storageVolume.PersistentVolumeClaim.ClaimName = instance.Status.PvcName

				c := kubernetes.FindContainerByNameOrCreate(spec, actions.ServerDeploymentName)
				storageVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(c, storageVolumeName)
				storageVolumeMount.MountPath = "/var/run/attestations"
				return nil
			},
			Managed: &v1.PodSpec{
				Volumes: []v1.Volume{{Name: storageVolumeName}},
				Containers: []v1.Container{{
					Name:         actions.ServerDeploymentName,
					VolumeMounts: []v1.VolumeMount{{Name: storageVolumeName}},
				}},
			},
		})(&dp.Spec.Template.Spec); err != nil {
			return err
		}

		return nil
	}
}

func ensureRedisParams() func(*redis.RedisOptions, *v1.Container) {
	return func(options *redis.RedisOptions, container *v1.Container) {
		container.Args = append(container.Args, "--search_index.storage_provider", "redis")
		container.Args = append(container.Args, "--redis_server.address", options.Host)

		if options.Port != "" {
			container.Args = append(container.Args, "--redis_server.port", options.Port)
		}

		if options.Password != "" {
			container.Args = append(container.Args, "--redis_server.password", options.Password)
		}
		if options.TlsEnabled {
			// boolean flag MUST be without parameter (default value) or use the equal sign (https://github.com/spf13/pflag?tab=readme-ov-file#command-line-flag-syntax)
			container.Args = append(container.Args, "--redis_server.enable-tls=true")
		}
	}
}

func ensureMysqlParams() func(string, *v1.Container) {
	return func(url string, container *v1.Container) {
		container.Args = append(container.Args, "--search_index.storage_provider", "mysql")
		container.Args = append(container.Args, "--search_index.mysql.dsn", url)
		container.Args = append(container.Args, "--search_index.mysql.max_open_connections", "30")
		container.Args = append(container.Args, "--search_index.mysql.max_idle_connections", "10")
		container.Args = append(container.Args, "--search_index.mysql.conn_max_lifetime", "10m")
		container.Args = append(container.Args, "--search_index.mysql.conn_max_idletime", "2m")
	}
}
