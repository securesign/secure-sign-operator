package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	cutils "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const (
	storageVolumeName = "storage"
	configVolumeMount = "/config"
	redisConfPath     = configVolumeMount + "/redis.conf"
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
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)
	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)
	caPath, err := tls.CAPath(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.RedisDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureRedisDeployment(instance, actions.RBACRedisName, labels),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.RedisDeploymentName, actions.RedisDeploymentName),
		ensure.Optional(statusTLS(instance).CertRef != nil, i.ensureTLS(statusTLS(instance), caPath)),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s deployment: %w", actions.RedisDeploymentName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Redis created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}

func (i deployAction) ensureRedisDeployment(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.RedisDeploymentName)
		container.Image = images.Registry.Get(images.RekorRedis)

		if err := i.ensurePassword(instance, container); err != nil {
			return err
		}

		port := kubernetes.FindPortByNameOrCreate(container, "redis")
		port.Protocol = core.ProtocolTCP
		port.ContainerPort = actions.RedisDeploymentPort

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.Exec == nil {
			container.ReadinessProbe.Exec = &core.ExecAction{}
		}

		container.ReadinessProbe.Exec.Command = []string{
			"/bin/sh",
			"-c",
			"-i",
			"test $(redis-cli -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'",
		}
		container.ReadinessProbe.InitialDelaySeconds = 5

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.Exec == nil {
			container.LivenessProbe.Exec = &core.ExecAction{}
		}
		container.LivenessProbe.Exec.Command = []string{
			"/bin/sh",
			"-c",
			"-i",
			"test $(redis-cli -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'",
		}
		container.LivenessProbe.InitialDelaySeconds = 10

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
		volumeMount.MountPath = "/data"

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, storageVolumeName)
		if volume.EmptyDir == nil {
			volume.EmptyDir = &core.EmptyDirVolumeSource{}
		}

		return nil
	}
}

func (i deployAction) ensurePassword(instance *rhtasv1alpha1.Rekor, container *core.Container) error {
	if instance.Status.SearchIndex.DbPasswordRef == nil {
		return errors.New("search index db password not found")
	}

	passwordEnv := kubernetes.FindEnvByNameOrCreate(container, "REDIS_PASSWORD")
	if passwordEnv.ValueFrom == nil {
		passwordEnv.ValueFrom = &core.EnvVarSource{}
	}
	if passwordEnv.ValueFrom.SecretKeyRef == nil {
		passwordEnv.ValueFrom.SecretKeyRef = &core.SecretKeySelector{}
	}
	passwordEnv.ValueFrom.SecretKeyRef.Name = instance.Status.SearchIndex.DbPasswordRef.Name
	passwordEnv.ValueFrom.SecretKeyRef.Key = instance.Status.SearchIndex.DbPasswordRef.Key
	return nil
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1alpha1.TLS, caPath string) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		if err := deployment.TLS(tlsConfig, actions.RedisDeploymentName)(dp); err != nil {
			return err
		}

		dbConfig := []string{
			fmt.Sprintf("tls-port %d", actions.RedisDeploymentPort),
			// disable non-tls ports
			"port 0",
			fmt.Sprintf("tls-cert-file %s", tls.TLSCertPath),
			fmt.Sprintf("tls-key-file %s", tls.TLSKeyPath),
			fmt.Sprintf("tls-ca-cert-file %s", caPath),
			// disable client authentication
			"tls-auth-clients no",
		}

		config := kubernetes.FindVolumeByNameOrCreate(&dp.Spec.Template.Spec, "config")
		if config.EmptyDir == nil {
			config.EmptyDir = &core.EmptyDirVolumeSource{}
		}

		init := kubernetes.FindInitContainerByNameOrCreate(&dp.Spec.Template.Spec, "enable-tls")
		init.Image = images.Registry.Get(images.RekorRedis)
		initVolumeName := kubernetes.FindVolumeMountByNameOrCreate(init, config.Name)
		initVolumeName.MountPath = configVolumeMount
		init.Command = []string{"/bin/bash", "-c"}
		init.Args = []string{
			fmt.Sprintf("cp $REDIS_CONF %s\n", redisConfPath),
		}
		for _, v := range dbConfig {
			init.Args[0] += fmt.Sprintf("echo \"%s\" >> %s\n", v, redisConfPath)
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.RedisDeploymentName)
		container.Image = images.Registry.Get(images.RekorRedis)
		containerVolumeName := kubernetes.FindVolumeMountByNameOrCreate(container, config.Name)
		containerVolumeName.MountPath = configVolumeMount

		configPathEnv := kubernetes.FindEnvByNameOrCreate(container, "REDIS_CONF")
		configPathEnv.Value = redisConfPath

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.Exec == nil {
			container.ReadinessProbe.Exec = &core.ExecAction{}
		}

		container.ReadinessProbe.Exec.Command = []string{
			"/bin/sh",
			"-c",
			"-i",
			fmt.Sprintf("test $(redis-cli --tls --cacert %s -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'", caPath),
		}

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.Exec == nil {
			container.LivenessProbe.Exec = &core.ExecAction{}
		}

		container.LivenessProbe.Exec.Command = []string{
			"/bin/sh",
			"-c",
			"-i",
			fmt.Sprintf("test $(redis-cli --tls --cacert %s -h 127.0.0.1 -a $REDIS_PASSWORD ping) = 'PONG'", caPath),
		}

		return nil
	}
}
