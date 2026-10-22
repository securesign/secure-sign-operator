package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1 "github.com/securesign/operator/api/v1"
	pkcs11helpers "github.com/securesign/operator/internal/controller/common/pkcs11"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/serviceresolver"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const containerName = "fulcio-server"

func NewDeployAction() action.Action[*rhtasv1.Fulcio] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	ctlogUrl, err := serviceresolver.ResolveInternalServiceUrl(ctx, i.Client, instance.Spec.Ctlog, instance.Namespace, &rhtasv1.CTlog{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve CTLog url: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for CTLog service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	dp, err := traits.Apply(
		appsv1ac.Deployment(DeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels),
		i.createFulcioDeployment(instance, ctlogUrl),
		traits.PodRequirements(instance.Spec.PodRequirements, containerName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{containerName}, "@fulcio-legacy-grpc-socket"),
			traits.GODEBUG(instance.GetAnnotations(), containerName),
			traits.TrustedCA(instance.GetTrustedCA(), containerName),
			traits.SecurityContexts(containerName),
			traits.ServiceAccount(RBACName),
			traits.Auth(instance.Spec.Auth, containerName),
			traits.PodExtensions(instance.Spec.PodExtensions, containerName),
		),
	)
	if err != nil {
		if errors.Is(err, traits.ErrPodExtensionConflict) {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
		return i.Error(ctx, fmt.Errorf("could not build Fulcio deployment: %w", err), instance, metav1.Condition{
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
		return i.Error(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Failed to create deployment: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	if changed {
		if instance.Spec.Signer.Type != rhtasv1.SignerTypePKCS11 {
			meta.RemoveStatusCondition(&instance.Status.Conditions, PKCS11Condition)
		}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.ReadyCondition,
			Status: metav1.ConditionFalse, Reason: state.Creating.String(), Message: "Deployment created",
			ObservedGeneration: instance.Generation})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

// createFulcioDeployment builds a fresh desired spec before converting it to a
// typed apply configuration. Building from scratch is important for SSA: fields
// removed from the desired state can then be pruned instead of being copied from
// the live object as they were with CreateOrUpdate.
func (i deployAction) createFulcioDeployment(instance *rhtasv1.Fulcio, ctlogURL string) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {

		ports := []*corev1ac.ContainerPortApplyConfiguration{
			{
				Name:          ptr.To("http"),
				ContainerPort: ptr.To(int32(5555)),
				Protocol:      ptr.To(core.ProtocolTCP),
			},
			{
				Name:          ptr.To("grpc"),
				ContainerPort: ptr.To(int32(5554)),
				Protocol:      ptr.To(core.ProtocolTCP),
			},
		}
		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			ports = append(ports, &corev1ac.ContainerPortApplyConfiguration{
				Name:          ptr.To("monitoring"),
				ContainerPort: ptr.To(int32(2112)),
				Protocol:      ptr.To(core.ProtocolTCP),
			})
		}

		args := []string{
			"serve",
			"--port=5555",
			"--grpc-port=5554",
			fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
			fmt.Sprintf("--ct-log-url=%s", ctlogURL),
		}
		if fips.Enabled() {
			args = append(args, "--client-signing-algorithms", fips.ClientSigningAlgorithms)
		}

		container := corev1ac.Container().
			WithName(containerName).
			WithImage(images.Registry.Get(images.FulcioServer)).
			WithPorts(ports...).
			WithArgs(args...).
			WithLivenessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(5555))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(5555))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(5555)).
					WithScheme(core.URISchemeHTTP)).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		podSpec := corev1ac.PodSpec().
			WithAutomountServiceAccountToken(true).
			WithContainers(container)

		switch instance.Spec.Signer.Type {
		case rhtasv1.SignerTypePKCS11:
			if err := i.ensurePKCS11Deployment(instance)(podSpec); err != nil {
				return fmt.Errorf("ensure PKCS#11 deployment: %w", err)
			}
		case rhtasv1.SignerTypeKMS:
			if err := i.ensureKMSCADeployment(instance)(podSpec); err != nil {
				return fmt.Errorf("ensure KMS CA deployment: %w", err)
			}
		default:
			if err := i.ensureFileCADeployment(instance)(podSpec); err != nil {
				return fmt.Errorf("ensure file CA deployment: %w", err)
			}
		}

		dp.
			WithSpec(appsv1ac.DeploymentSpec().
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
				WithStrategy(appsv1ac.DeploymentStrategy().
					WithType(apps.RollingUpdateDeploymentStrategyType)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(dp.Labels).
					WithSpec(podSpec)))

		return nil
	}
}

func ensureSignerVolumes(container *corev1ac.ContainerApplyConfiguration, template *corev1ac.PodSpecApplyConfiguration, instance *rhtasv1.Fulcio) {
	container.WithVolumeMounts(
		corev1ac.VolumeMount().
			WithName("fulcio-cert").
			WithMountPath("/var/run/fulcio-secrets").
			WithReadOnly(true),
		corev1ac.VolumeMount().
			WithName("fulcio-config").
			WithMountPath("/etc/fulcio-config"),
		corev1ac.VolumeMount().
			WithName("oidc-info").
			WithMountPath("/var/run/fulcio"),
	)

	template.WithVolumes(
		corev1ac.Volume().
			WithName("fulcio-cert").
			WithProjected(corev1ac.ProjectedVolumeSource().
				WithDefaultMode(0644).
				WithSources(corev1ac.VolumeProjection().
					WithSecret(corev1ac.SecretProjection().
						WithName(instance.Status.Certificate.CARef.LocalObjectReference.Name).
						WithItems(corev1ac.KeyToPath().
							WithKey(instance.Status.Certificate.CARef.Key).
							WithPath(fulcioCertPemKey))))),
		corev1ac.Volume().
			WithName("fulcio-config").
			WithConfigMap(corev1ac.ConfigMapVolumeSource().
				WithName(instance.Status.ServerConfigRef.Name).
				WithDefaultMode(0644)),
		corev1ac.Volume().
			WithName("oidc-info").
			WithProjected(corev1ac.ProjectedVolumeSource().
				WithDefaultMode(0644).
				WithSources(corev1ac.VolumeProjection().
					WithConfigMap(corev1ac.ConfigMapProjection().
						WithName("kube-root-ca.crt").
						WithItems(corev1ac.KeyToPath().
							WithKey(kubeRootCACertKey).
							WithPath(kubeRootCACertKey).
							WithMode(0666))))),
	)
}

func (i deployAction) ensureFileCADeployment(instance *rhtasv1.Fulcio) func(*corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if instance.Status.ServerConfigRef == nil {
			return errors.New("server config ref is not specified")
		}
		if instance.Status.Certificate == nil {
			return errors.New("certificate config is not specified")
		}
		if instance.Status.Certificate.PrivateKeyRef == nil {
			return errors.New("private key secret is not specified")
		}

		if instance.Status.Certificate.CARef == nil {
			return errors.New("CA secret is not specified")
		}

		container := kubernetes.FindContainerByName(podSpec, containerName)
		if container == nil {
			return errors.New("container not found")
		}

		if instance.Status.Certificate.PrivateKeyPasswordRef != nil {
			container.WithEnv(corev1ac.EnvVar().
				WithName("PASSWORD").
				WithValueFrom(corev1ac.EnvVarSource().
					WithSecretKeyRef(corev1ac.SecretKeySelector().
						WithKey(instance.Status.Certificate.PrivateKeyPasswordRef.Key).WithName(instance.Status.Certificate.PrivateKeyPasswordRef.Name))))
			container.WithArgs("--fileca-key-passwd", "$(PASSWORD)")
		}

		ensureSignerVolumes(container, podSpec, instance)

		for i := range podSpec.Volumes {
			volume := &podSpec.Volumes[i]
			if volume.Name == nil || *volume.Name != "fulcio-cert" {
				continue
			}
			if volume.Projected == nil {
				return errors.New("fulcio-cert projected volume is not configured")
			}

			volume.Projected.WithSources(corev1ac.VolumeProjection().
				WithSecret(corev1ac.SecretProjection().
					WithName(instance.Status.Certificate.PrivateKeyRef.Name).
					WithItems(corev1ac.KeyToPath().
						WithKey(instance.Status.Certificate.PrivateKeyRef.Key).
						WithPath("key.pem"))))
		}

		container.WithArgs(
			"--ca=fileca",
			"--fileca-key",
			"/var/run/fulcio-secrets/key.pem",
			"--fileca-cert",
			"/var/run/fulcio-secrets/cert.pem",
		)

		return nil
	}
}

func (i deployAction) ensureKMSCADeployment(instance *rhtasv1.Fulcio) func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if instance.Status.ServerConfigRef == nil {
			return errors.New("server config ref is not specified")
		}
		if instance.Status.Certificate == nil {
			return errors.New("certificate config is not specified")
		}
		if instance.Status.Certificate.CARef == nil {
			return errors.New("CA secret is not specified")
		}
		if instance.Spec.Signer.Kms == nil {
			return errors.New("kms config is required when type is kms")
		}

		container := kubernetes.FindContainerByName(podSpec, containerName)
		if container == nil {
			return errors.New("container not found")
		}

		ensureSignerVolumes(container, podSpec, instance)

		container.WithArgs(
			"--ca=kmsca",
			"--kms-resource", instance.Spec.Signer.Kms.KeyResource,
			"--kms-cert-chain-path", "/var/run/fulcio-secrets/cert.pem",
		)

		return nil
	}
}

func (i deployAction) ensurePKCS11Deployment(instance *rhtasv1.Fulcio) func(*corev1ac.PodSpecApplyConfiguration) error {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if instance.Spec.Signer.PKCS11 == nil {
			return errors.New("PKCS#11 config not specified")
		}
		if instance.Status.ServerConfigRef == nil {
			return errors.New("server config ref is not specified")
		}
		if instance.Status.Certificate == nil || instance.Status.Certificate.CARef == nil {
			return errors.New("CA certificate reference not yet resolved")
		}

		container := kubernetes.FindContainerByName(podSpec, containerName)
		if container == nil {
			return errors.New("container not found")
		}

		pkcs11Cfg := instance.Spec.Signer.PKCS11
		configRef := pkcs11Cfg.ConfigRef

		container.WithArgs(
			"--ca=pkcs11ca",
			fmt.Sprintf("--pkcs11-config-path=%s/%s", PKCS11ConfigMountPath, configRef.Key),
			fmt.Sprintf("--aws-hsm-root-ca-path=%s/%s",
				PKCS11CertMountPath, instance.Status.Certificate.CARef.Key),
			fmt.Sprintf("--hsm-caroot-id=%d", pkcs11Cfg.KeyID),
		)

		// PKCS#11-specific volume mounts

		container.WithVolumeMounts(
			corev1ac.VolumeMount().
				WithName(PKCS11ConfigVolumeName).
				WithMountPath(PKCS11ConfigMountPath).
				WithReadOnly(true),
			corev1ac.VolumeMount().
				WithName(PKCS11CertVolumeName).
				WithMountPath(PKCS11CertMountPath).
				WithReadOnly(true),
			corev1ac.VolumeMount().
				WithName("fulcio-config").
				WithMountPath("/etc/fulcio-config"),
			corev1ac.VolumeMount().
				WithName("oidc-info").
				WithMountPath("/var/run/fulcio"),
		)

		pkcs11helpers.ApplyHSMResources(podSpec, container, instance.Spec.Volumes)

		podSpec.WithVolumes(
			// PKCS#11-specific volumes
			corev1ac.Volume().
				WithName(PKCS11ConfigVolumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(configRef.Name).
					WithDefaultMode(0644).
					WithItems(corev1ac.KeyToPath().
						WithKey(configRef.Key).
						WithPath(configRef.Key))),
			corev1ac.Volume().
				WithName(PKCS11CertVolumeName).
				WithSecret(corev1ac.SecretVolumeSource().
					WithSecretName(instance.Status.Certificate.CARef.Name).
					WithDefaultMode(0644).
					WithItems(corev1ac.KeyToPath().
						WithKey(instance.Status.Certificate.CARef.Key).
						WithPath(instance.Status.Certificate.CARef.Key))),

			// Shared volumes (same as file mode)
			corev1ac.Volume().
				WithName("fulcio-config").
				WithConfigMap(corev1ac.ConfigMapVolumeSource().
					WithName(instance.Status.ServerConfigRef.Name).
					WithDefaultMode(0644)),
			corev1ac.Volume().
				WithName("oidc-info").
				WithProjected(corev1ac.ProjectedVolumeSource().
					WithDefaultMode(0644).
					WithSources(corev1ac.VolumeProjection().
						WithConfigMap(corev1ac.ConfigMapProjection().
							WithName("kube-root-ca.crt").
							WithItems(corev1ac.KeyToPath().
								WithKey(kubeRootCACertKey).
								WithPath(kubeRootCACertKey).
								WithMode(0666))))),
		)

		return nil
	}
}
