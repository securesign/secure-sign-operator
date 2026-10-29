package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	chainVolumeName      = "tsa-cert-chain"
	fileSignerVolumeName = "tsa-file-signer-config"
	tinkSignerVolumeName = "tsa-tink-signer-config"
	ntpConfigVolumeName  = "ntp-config"
	certChainMountPath   = constants.SecretMountPath + "/certificate_chain"
	fileSignerMountPath  = constants.SecretMountPath + "/file_signer"
	tinkSignerMountPath  = constants.SecretMountPath + "/tink_signer"
	NtpMountPath         = constants.SecretMountPath + "/ntp_config"
)

type deployAction struct {
	action.BaseAction
}

func NewDeployAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &deployAction{}
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1.TimestampAuthority) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {

	dp, err := traits.Apply(
		appsv1ac.Deployment(DeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(ComponentName, DeploymentName, instance.Name)),
		i.ensureDeployment(instance),
		traits.PodRequirements(instance.Spec.PodRequirements, DeploymentName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{DeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), DeploymentName),
			traits.TrustedCA(instance.GetTrustedCA(), DeploymentName),
			traits.SecurityContexts(DeploymentName),
			traits.ServiceAccount(RBACName),
			traits.Auth(instance.Spec.Auth, DeploymentName),
			traits.PodExtensions(instance.Spec.PodExtensions, DeploymentName),
		),
	)
	if err != nil {
		if errors.Is(err, traits.ErrPodExtensionConflict) {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
		return i.Error(ctx, fmt.Errorf("could not build Timestamp Authority deployment: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Failed to build deployment: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Timestamp Authority server: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Failed to create deployment: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "TSA server deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func (i deployAction) ensureDeployment(instance *rhtasv1.TimestampAuthority) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {

		container := corev1ac.Container().
			WithName(DeploymentName).
			WithImage(images.Registry.Get(images.TimestampAuthority)).
			WithCommand("timestamp-server", "serve").
			WithArgs(
				"--host", "0.0.0.0",
				"--port", "3000",
				"--log-type", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod)),
				"--certificate-chain-path", fmt.Sprintf("%s/certificate-chain.pem", certChainMountPath),
				// disable-ntp-monitoring is a boolean flag, so we need to use the `=` assignment operator
				fmt.Sprintf("--disable-ntp-monitoring=%t", !utils.IsEnabled(instance.Spec.NTPMonitoring.Enabled))).
			WithPorts(corev1ac.ContainerPort().
				WithName("3000-tcp").
				WithContainerPort(3000).
				WithProtocol(core.ProtocolTCP)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(chainVolumeName).
				WithMountPath(certChainMountPath).
				WithReadOnly(true)).
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
					WithPath("/ping").
					WithPort(intstr.FromInt32(3000))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			// StartupProbe verifies TSA is fully initialized by checking certificate chain endpoint
			// This runs only at startup, allowing heavy checks without impacting ongoing probes
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/api/v1/timestamp/certchain").
					WithPort(intstr.FromInt32(3000))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		podSpec := corev1ac.PodSpec().
			WithVolumes(corev1ac.Volume().
				WithName(chainVolumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(instance.Status.Signer.CertificateChainRef.Name).
					WithItems(corev1ac.KeyToPath().
						WithKey(instance.Status.Signer.CertificateChainRef.Key).
						WithPath("certificate-chain.pem"))))

		if instance.Spec.MaxRequestBodySize != nil {
			container.WithArgs("--max-request-body-size", fmt.Sprintf("%d", *instance.Spec.MaxRequestBodySize))
		}

		if utils.IsEnabled(instance.Spec.NTPMonitoring.Enabled) && instance.Status.NtpConfigRef != nil {
			container.WithArgs("--ntp-monitoring", fmt.Sprintf("%s/ntp-config.yaml", NtpMountPath))
			container.WithVolumeMounts(corev1ac.VolumeMount().
				WithName(ntpConfigVolumeName).
				WithMountPath(NtpMountPath).
				WithReadOnly(true))

			podSpec.WithVolumes(corev1ac.Volume().
				WithName(ntpConfigVolumeName).
				WithConfigMap(corev1ac.ConfigMapVolumeSource().
					WithName(instance.Status.NtpConfigRef.Name)))
		}

		switch instance.Spec.Signer.Type {
		case rhtasv1.SignerTypeKMS:
			container.WithArgs("--timestamp-signer", "kms", "--kms-key-resource", instance.Spec.Signer.Kms.KeyResource)
		case rhtasv1.SignerTypeTink:
			container.
				WithArgs("--timestamp-signer", "tink",
					"--tink-key-resource", instance.Spec.Signer.Tink.KeyResource,
					"--tink-keyset-path", fmt.Sprintf("%s/encryptedKeySet", tinkSignerMountPath)).
				WithVolumeMounts(corev1ac.VolumeMount().
					WithName(tinkSignerVolumeName).
					WithMountPath(tinkSignerMountPath).
					WithReadOnly(true))

			podSpec.WithVolumes(corev1ac.Volume().
				WithName(tinkSignerVolumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(instance.Spec.Signer.Tink.KeysetRef.Name).
					WithItems(corev1ac.KeyToPath().
						WithKey(instance.Spec.Signer.Tink.KeysetRef.Key).
						WithPath("encryptedKeySet"))))

			if strings.HasPrefix(instance.Spec.Signer.Tink.KeyResource, "hcvault://") {
				container.WithArgs("--tink-hcvault-token", "$(VAULT_TOKEN)")
			}

		default:
			container.WithArgs("--timestamp-signer", "file", "--file-signer-key-path", fmt.Sprintf("%s/private_key.pem", fileSignerMountPath))
			container.WithVolumeMounts(corev1ac.VolumeMount().
				WithName(fileSignerVolumeName).
				WithMountPath(fileSignerMountPath).
				WithReadOnly(true))

			podSpec.WithVolumes(corev1ac.Volume().
				WithName(fileSignerVolumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).
					WithItems(corev1ac.KeyToPath().
						WithKey(instance.Status.Signer.FileSigner.PrivateKeyRef.Key).
						WithPath("private_key.pem"))))

			if instance.Status.Signer.FileSigner.PasswordRef != nil {
				container.WithArgs("--file-signer-passwd", "$(SIGNER_PASSWORD)").
					WithEnv(corev1ac.EnvVar().
						WithName("SIGNER_PASSWORD").
						WithValueFrom(corev1ac.EnvVarSource().
							WithSecretKeyRef(corev1ac.SecretKeySelector().
								WithName(instance.Status.Signer.FileSigner.PasswordRef.Name).
								WithKey(instance.Status.Signer.FileSigner.PasswordRef.Key))))
			}
		}

		dp.
			WithSpec(appsv1ac.DeploymentSpec().
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
				WithStrategy(appsv1ac.DeploymentStrategy().
					WithType(apps.RollingUpdateDeploymentStrategyType)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(dp.Labels).
					WithSpec(podSpec.WithContainers(container))))

		return nil
	}
}
