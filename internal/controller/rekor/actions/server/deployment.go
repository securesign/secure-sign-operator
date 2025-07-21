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
	utils2 "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	"k8s.io/utils/ptr"

	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/utils"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)
	labels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	insCopy := instance.DeepCopy()
	if insCopy.Spec.Trillian.Address == "" {
		insCopy.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", actions2.LogserverDeploymentName, instance.Namespace)
	}
	i.Logger.V(1).Info("trillian logserver", "address", insCopy.Spec.Trillian.Address)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ServerDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureServerDeployment(insCopy, actions.RBACName, labels),
		deployment.PodRequirements(insCopy.Spec.PodRequirements, actions.ServerDeploymentName),
		i.ensureAttestation(insCopy),
		ensure.ControllerReference[*v2.Deployment](instance, i.Client),
		ensure.Labels[*v2.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Auth(actions.ServerDeploymentName, instance.Spec.Auth),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.ServerDeploymentName),
		ensure.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian()),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		// Regenerate secret with public key
		instance.Status.PublicKeyRef = nil

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "DeploymentUpdated", "Deployment updated: %s", instance.Name)
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureServerDeployment(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string) func(*v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		switch {
		case instance.Status.ServerConfigRef == nil:
			return fmt.Errorf("CreateRekorDeployment: %w", utils.ErrServerConfigNotSpecified)
		case instance.Status.TreeID == nil:
			return fmt.Errorf("CreateRekorDeployment: %w", utils.ErrTreeNotSpecified)
		case instance.Spec.Trillian.Address == "":
			return fmt.Errorf("CreateRekorDeployment: %w", utils.ErrTrillianAddressNotSpecified)
		case instance.Spec.Trillian.Port == nil:
			return fmt.Errorf("CreateRekorDeployment: %w", utils.ErrTrillianPortNotSpecified)
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
			"--trillian_log_server.address", instance.Spec.Trillian.Address,
			"--trillian_log_server.port", strconv.Itoa(int(*instance.Spec.Trillian.Port)),
			"--trillian_log_server.sharding_config", "/sharding/sharding-config.yaml",

			"--rekor_server.address", "0.0.0.0",
			"--enable_retrieve_api", "true",
			"--trillian_log_server.tlog_id", strconv.FormatInt(*instance.Status.TreeID, 10),
			"--log_type", utils2.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod)),
		}

		// KMS memory
		if instance.Spec.Signer.KMS == "memory" {
			args = append(args, "--rekor_server.signer", "memory")
		}

		// KMS secret
		if instance.Spec.Signer.KMS == "secret" || instance.Spec.Signer.KMS == "" { //nolint:goconst
			if instance.Status.Signer.KeyRef == nil {
				return utils.ErrSignerKeyNotSpecified
			}
			var volumeName = "rekor-private-key-volume"
			privateVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
			if privateVolume.Secret == nil {
				privateVolume.Secret = &v1.SecretVolumeSource{}
			}
			privateVolume.Secret.SecretName = instance.Status.Signer.KeyRef.Name
			privateVolume.Secret.Items = []v1.KeyToPath{
				{
					Key:  instance.Status.Signer.KeyRef.Key,
					Path: "private",
				},
			}

			volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
			volumeMount.MountPath = "/key"
			volumeMount.ReadOnly = true

			args = append(args, "--rekor_server.signer", "/key/private")

			// Add signer password
			if instance.Status.Signer.PasswordRef != nil {
				args = append(args, "--rekor_server.signer-passwd", "$(SIGNER_PASSWORD)")
				env := kubernetes.FindEnvByNameOrCreate(container, "SIGNER_PASSWORD")
				env.ValueFrom = &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						Key: instance.Status.Signer.PasswordRef.Key,
						LocalObjectReference: v1.LocalObjectReference{
							Name: instance.Status.Signer.PasswordRef.Name,
						},
					},
				}
			}
		}

		//TODO mount additional ENV variables and secrets to enable cloud KMS service
		container.Args = args
		if err := searchIndex.EnsureSearchIndex(instance, ensureRedisParams(), ensureMysqlParams())(container); err != nil {
			return err
		}

		serverPort := kubernetes.FindPortByNameOrCreate(container, "rekor-server")
		serverPort.ContainerPort = 3000

		if instance.Spec.Monitoring.Enabled {
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
		container.LivenessProbe.InitialDelaySeconds = 30

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &v1.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &v1.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/ping"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.ReadinessProbe.InitialDelaySeconds = 10

		return nil
	}
}

func (i deployAction) ensureTlsTrillian() func(*v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ServerDeploymentName)

		container.Args = append(container.Args, "--trillian_log_server.tls", "true")
		return nil
	}
}

func (i deployAction) ensureAttestation(instance *rhtasv1alpha1.Rekor) func(*v2.Deployment) error {
	const storageVolumeName = "storage"
	return func(dp *v2.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ServerDeploymentName)
		enabled := ptr.Deref(instance.Spec.Attestations.Enabled, false)

		container.Args = append(container.Args, "--enable_attestation_storage", strconv.FormatBool(enabled))

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
		if enabledFileAttestationStorage(instance) {
			storageVolume := kubernetes.FindVolumeByNameOrCreate(&dp.Spec.Template.Spec, storageVolumeName)
			if storageVolume.PersistentVolumeClaim == nil {
				storageVolume.PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{}
			}
			storageVolume.PersistentVolumeClaim.ClaimName = instance.Status.PvcName

			storageVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
			storageVolumeMount.MountPath = "/var/run/attestations"
		} else {
			// other storage bucket options

			// remove unused storage volume
			kubernetes.RemoveVolumeByName(&dp.Spec.Template.Spec, storageVolumeName)
			kubernetes.RemoveVolumeMountByName(container, storageVolumeName)
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
			container.Args = append(container.Args, "--redis_server.enable-tls", "true")
		}
	}
}

func ensureMysqlParams() func(string, *v1.Container) {
	return func(url string, container *v1.Container) {
		container.Args = append(container.Args, "--search_index.storage_provider", "mysql")
		container.Args = append(container.Args, "--search_index.mysql.dsn", url)
	}
}
