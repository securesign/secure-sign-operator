package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	"k8s.io/apimachinery/pkg/api/meta"

	rhtasv1 "github.com/securesign/operator/api/v1"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	volumeName    = "keys"
	containerName = "ctlog"
)

func NewDeployAction() action.Action[*rhtasv1.CTlog] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureDeployment(instance, RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), containerName),
		deployment.PodRequirements(instance.Spec.PodRequirements, containerName),
		deployment.PodSecurityContext(),
		ensure.Optional(
			ctlogutils.TlsEnabled(instance),
			i.ensureTLS(instance.Status.TLS, containerName),
		),
		ensure.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian(ctx, instance)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ctlog server deployment: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureDeployment(instance *rhtasv1.CTlog, sa string, labels map[string]string) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		switch {
		case instance.Status.ServerConfigRef == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrServerConfigNotSpecified)
		case instance.Status.TreeID == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrTreeNotSpecified)
		case resolveTrillianAddress(instance) == "":
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrTrillianAddressNotSpecified)
		case instance.Spec.Trillian.Port == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrTrillianPortNotSpecified)
		}

		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
		if volume.Secret == nil {
			volume.Secret = &core.SecretVolumeSource{}
		}
		volume.Secret.SecretName = instance.Status.ServerConfigRef.Name

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		container.Image = images.Registry.Get(images.CTLog)

		serverPort := kubernetes.FindPortByNameOrCreate(container, "server")
		serverPort.ContainerPort = ServerTargetPort
		serverPort.Protocol = core.ProtocolTCP

		appArgs := []string{
			"--http_endpoint=0.0.0.0:" + strconv.Itoa(ServerTargetPort),
			"--log_config=/ctfe-keys/config",
			"--alsologtostderr",
		}

		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			appArgs = append(appArgs, "--metrics_endpoint=0.0.0.0:"+strconv.Itoa(MetricsPort))
			metricsPort := kubernetes.FindPortByNameOrCreate(container, "metrics")
			metricsPort.ContainerPort = MetricsPort
			metricsPort.Protocol = core.ProtocolTCP
		}

		container.Args = appArgs
		if instance.Spec.MaxCertChainSize != nil {
			container.Args = append(container.Args, "--max_cert_chain_size", fmt.Sprintf("%d", *instance.Spec.MaxCertChainSize))
		}

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
		volumeMount.MountPath = "/ctfe-keys"
		volumeMount.ReadOnly = true

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = constants.HealthzPath
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(ServerTargetPort)
		container.LivenessProbe.InitialDelaySeconds = 0
		container.LivenessProbe.PeriodSeconds = 10
		container.LivenessProbe.TimeoutSeconds = 1
		container.LivenessProbe.FailureThreshold = 3

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = constants.HealthzPath
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(ServerTargetPort)
		container.ReadinessProbe.InitialDelaySeconds = 0
		container.ReadinessProbe.PeriodSeconds = 10
		container.ReadinessProbe.TimeoutSeconds = 1
		container.ReadinessProbe.FailureThreshold = 3

		if container.StartupProbe == nil {
			container.StartupProbe = &core.Probe{}
		}
		if container.StartupProbe.HTTPGet == nil {
			container.StartupProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.StartupProbe.HTTPGet.Path = constants.HealthzPath
		container.StartupProbe.HTTPGet.Port = intstr.FromInt32(ServerTargetPort)
		container.StartupProbe.PeriodSeconds = 5
		container.StartupProbe.TimeoutSeconds = 5
		container.StartupProbe.FailureThreshold = 12

		return nil
	}
}

func (i deployAction) ensureTlsTrillian(ctx context.Context, instance *rhtasv1.CTlog) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		caPath, err := tls.CAPath(ctx, i.Client, instance)
		if err != nil {
			return fmt.Errorf("failed to get CA path: %w", err)
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)

		container.Args = append(container.Args, "--trillian_tls_ca_cert_file", caPath)
		return nil
	}
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1.TLS, name string) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		if err := deployment.TLS(tlsConfig, name)(dp); err != nil {
			return err
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, name)

		container.Args = append(container.Args, "--tls_certificate", tls.TLSCertPath)
		container.Args = append(container.Args, "--tls_key", tls.TLSKeyPath)

		if container.ReadinessProbe != nil {
			container.ReadinessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		if container.LivenessProbe != nil {
			container.LivenessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		if container.StartupProbe != nil {
			container.StartupProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		return nil
	}
}
