package actions

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	"k8s.io/apimachinery/pkg/api/meta"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/common/pkcs11"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/controller/traits"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	labels := labels.For(ComponentName, DeploymentName, instance.Name)
	usePKCS11 := slices.ContainsFunc(instance.Spec.Logs, func(log rhtasv1.CTLogConfig) bool {
		return log.Signer != nil && log.Signer.Type == rhtasv1.SignerTypePKCS11
	})
	dp, err := traits.Apply(
		appsv1ac.Deployment(DeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels),
		i.ensureDeployment(instance),
		traits.PodRequirements(instance.Spec.PodRequirements, containerName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{containerName}),
			traits.GODEBUG(instance.GetAnnotations(), containerName),
			traits.TrustedCA(instance.GetTrustedCA(), containerName),
			traits.SecurityContexts(containerName),
			traits.ServiceAccount(RBACName),
			traits.Auth(instance.Spec.Auth, containerName),
			traits.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian(ctx, instance)),
			traits.Optional(ctlogutils.TlsEnabled(instance), i.ensureTLS(instance.Status.TLS, containerName)),
			traits.Optional(usePKCS11, i.ensurePKCS11(instance, containerName)),
			traits.PodExtensions(instance.Spec.PodExtensions, containerName),
		),
	)
	if err != nil {
		if errors.Is(err, traits.ErrPodExtensionConflict) {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
		return i.Error(ctx, fmt.Errorf("could not build CTlog deployment: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Failed to build deployment: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ctlog server deployment: %w", err), instance)
	}

	if changed {
		if !usePKCS11 {
			meta.RemoveStatusCondition(&instance.Status.Conditions, PKCS11Condition)
		}
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

func (i deployAction) ensureDeployment(instance *rhtasv1.CTlog) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		activeLog := ctlogutils.ActiveLogStatus(instance.Status.Logs)
		switch {
		case instance.Status.ServerConfigRef == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrServerConfigNotSpecified)
		case activeLog == nil || activeLog.LogId == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", ctlogutils.ErrTreeNotSpecified)
		}

		container := corev1ac.Container().
			WithName(containerName).
			WithImage(images.Registry.Get(images.CTLog)).
			WithPorts(corev1ac.ContainerPort().
				WithName("server").
				WithContainerPort(ServerTargetPort).
				WithProtocol(core.ProtocolTCP)).
			WithArgs(
				"--http_endpoint=0.0.0.0:"+strconv.Itoa(ServerTargetPort),
				"--log_config=/ctfe-keys/config",
				"--alsologtostderr",
			).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(volumeName).
				WithMountPath("/ctfe-keys").
				WithReadOnly(true)).
			WithLivenessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(ServerTargetPort))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(ServerTargetPort))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(ServerTargetPort))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			container.WithArgs("--metrics_endpoint", "0.0.0.0:"+strconv.Itoa(MetricsPort))
			container.WithPorts(corev1ac.ContainerPort().
				WithName("metrics").
				WithContainerPort(MetricsPort).
				WithProtocol(core.ProtocolTCP))
		}

		if instance.Spec.MaxCertChainSize != nil {
			container.WithArgs("--max_cert_chain_size", fmt.Sprintf("%d", *instance.Spec.MaxCertChainSize))
		}

		podSpec := corev1ac.PodSpec().
			WithContainers(container).
			WithVolumes(corev1ac.Volume().
				WithName(volumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(instance.Status.ServerConfigRef.Name).
					WithDefaultMode(0644)))

		dp.
			WithSpec(appsv1ac.DeploymentSpec().
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
				WithStrategy(appsv1ac.DeploymentStrategy().
					WithType(v1.RollingUpdateDeploymentStrategyType)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(dp.Labels).
					WithSpec(podSpec)))

		return nil
	}
}

func (i deployAction) ensurePKCS11(instance *rhtasv1.CTlog, container string) func(*corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		var pkcs11ModulePath string
		for _, log := range instance.Spec.Logs {
			if log.Signer != nil && log.Signer.Type == rhtasv1.SignerTypePKCS11 {
				if log.Signer.PKCS11 == nil {
					return fmt.Errorf("PKCS#11 config not yet resolved for log %s", log.Prefix)
				}
				// Validate that all PKCS#11 logs use the same full module path
				// (CTFE only supports a single process-wide --pkcs11_module_path)
				// Compare full paths to catch distinct source paths with the same basename
				if pkcs11ModulePath == "" {
					pkcs11ModulePath = log.Signer.PKCS11.ModulePath
				} else if pkcs11ModulePath != log.Signer.PKCS11.ModulePath {
					return fmt.Errorf(
						"conflicting PKCS#11 module paths: log %q uses %q but another log uses %q (all PKCS#11 logs must use the same module path)",
						log.Prefix, log.Signer.PKCS11.ModulePath, pkcs11ModulePath)
				}
			}
		}

		container := kubernetes.FindContainerByName(podSpec, container)
		if pkcs11ModulePath != "" {
			container.WithArgs("--pkcs11_module_path", fmt.Sprintf("%s/%s", constants.HSMLibMountPath, path.Base(pkcs11ModulePath)))
		}

		pkcs11.ApplyHSMResources(podSpec, container, instance.Spec.Volumes)

		return nil
	}
}
func (i deployAction) ensureTlsTrillian(ctx context.Context, instance *rhtasv1.CTlog) func(*corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		caPath, err := tls.CAPath(ctx, i.Client, instance)
		if err != nil {
			return fmt.Errorf("failed to get CA path: %w", err)
		}

		container := kubernetes.FindContainerByName(podSpec, containerName)

		container.WithArgs("--trillian_tls_ca_cert_file", caPath)
		return nil
	}
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1.TLS, name string) func(*corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if err := traits.TLS(tlsConfig, name)(podSpec); err != nil {
			return err
		}
		container := kubernetes.FindContainerByName(podSpec, name)

		container.
			WithArgs("--tls_certificate", tls.TLSCertPath).
			WithArgs("--tls_key", tls.TLSKeyPath)

		container.ReadinessProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)
		container.LivenessProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)
		container.StartupProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)

		return nil
	}
}
