package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1 "github.com/securesign/operator/api/v1"
	pkcs11helpers "github.com/securesign/operator/internal/controller/common/pkcs11"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/serviceresolver"
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
	var (
		result controllerutil.OperationResult
		err    error
	)

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

	// PodExtensions (user overrides) are applied first; operator ensure functions
	// run after, so operator-managed volumes take precedence over user collisions.
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		deployment.PodExtensions(instance.Spec.PodExtensions, containerName),
		i.ensureCommonDeployment(instance, RBACName, labels, ctlogUrl),
		ensure.OptionalToggle(instance.Spec.Signer.Type == rhtasv1.SignerTypeFile || instance.Spec.Signer.Type == "",
			ensure.Toggleable[*v1.Deployment]{
				Ensure: i.ensureFileCADeployment(instance),
				Managed: &v1.Deployment{
					Spec: v1.DeploymentSpec{
						Template: core.PodTemplateSpec{
							Spec: core.PodSpec{
								Containers: []core.Container{{
									Name: containerName,
									Env:  []core.EnvVar{{Name: "PASSWORD"}},
								}},
							},
						},
					},
				},
			}),
		ensure.Optional(instance.Spec.Signer.Type == rhtasv1.SignerTypePKCS11,
			i.ensurePKCS11Deployment(instance)),
		ensure.Optional(instance.Spec.Signer.Type == rhtasv1.SignerTypeKMS,
			i.ensureKMSCADeployment(instance)),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Auth(containerName, instance.Spec.Auth),
		// need to add Fulcio's unix domain socket used for the legacy gRPC server other way it will be
		// rest v1 api will be routed through proxy
		deployment.Proxy("@fulcio-legacy-grpc-socket"),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), containerName),
		deployment.PodRequirements(instance.Spec.PodRequirements, containerName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Failed to create deployment: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.ReadyCondition,
			Status: metav1.ConditionFalse, Reason: state.Creating.String(), Message: "Deployment created",
			ObservedGeneration: instance.Generation})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

// ensureCommonDeployment sets up the shared deployment scaffolding used by all signer modes:
// replicas, selector, labels, service account, ports, probes, and ct-log-url.
func (i deployAction) ensureCommonDeployment(instance *rhtasv1.Fulcio, sa string, labels map[string]string, ctlogUrl string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa
		template.Spec.AutomountServiceAccountToken = &[]bool{true}[0]

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		container.Image = images.Registry.Get(images.FulcioServer)

		http := kubernetes.FindPortByNameOrCreate(container, "http")
		http.ContainerPort = 5555
		http.Protocol = core.ProtocolTCP

		grpc := kubernetes.FindPortByNameOrCreate(container, "grpc")
		grpc.ContainerPort = 5554
		grpc.Protocol = core.ProtocolTCP

		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			monitoringPort := kubernetes.FindPortByNameOrCreate(container, "monitoring")
			monitoringPort.ContainerPort = 2112
			monitoringPort.Protocol = core.ProtocolTCP
		}

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = constants.HealthzPath
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(5555)
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
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(5555)
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
		container.StartupProbe.HTTPGet.Port = intstr.FromInt32(5555)
		container.StartupProbe.PeriodSeconds = 5
		container.StartupProbe.TimeoutSeconds = 5
		container.StartupProbe.FailureThreshold = 12

		container.Args = []string{
			"serve",
			"--port=5555",
			"--grpc-port=5554",
			fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
			fmt.Sprintf("--ct-log-url=%s", ctlogUrl),
		}

		if fips.Enabled() {
			container.Args = append(container.Args, "--client-signing-algorithms", fips.ClientSigningAlgorithms)
		}

		return nil
	}
}

func ensureSignerVolumes(container *core.Container, template *core.PodTemplateSpec, instance *rhtasv1.Fulcio) {
	certMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-cert")
	certMount.MountPath = "/var/run/fulcio-secrets"
	certMount.ReadOnly = true

	configMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-config")
	configMount.MountPath = "/etc/fulcio-config"

	oidcInfoMount := kubernetes.FindVolumeMountByNameOrCreate(container, "oidc-info")
	oidcInfoMount.MountPath = "/var/run/fulcio"

	config := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-config")
	config.VolumeSource = core.VolumeSource{
		ConfigMap: &core.ConfigMapVolumeSource{
			LocalObjectReference: core.LocalObjectReference{
				Name: instance.Status.ServerConfigRef.Name,
			},
			DefaultMode: ptr.To(int32(0644)),
		},
	}

	oidcInfo := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "oidc-info")
	oidcInfo.VolumeSource = core.VolumeSource{
		Projected: &core.ProjectedVolumeSource{
			DefaultMode: ptr.To(int32(0644)),
		},
	}
	oidcInfo.Projected.Sources = []core.VolumeProjection{
		{
			ConfigMap: &core.ConfigMapProjection{
				LocalObjectReference: core.LocalObjectReference{
					Name: "kube-root-ca.crt",
				},
				Items: []core.KeyToPath{
					{
						Key:  kubeRootCACertKey,
						Path: kubeRootCACertKey,
						Mode: ptr.To(int32(0666)),
					},
				},
			},
		},
	}

}

func (i deployAction) ensureFileCADeployment(instance *rhtasv1.Fulcio) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
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

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)
		template := &dp.Spec.Template

		// Clean up PKCS#11-specific volumes when switching back to file mode
		kubernetes.RemoveVolumeByName(&template.Spec, PKCS11ConfigVolumeName)
		kubernetes.RemoveVolumeByName(&template.Spec, PKCS11CertVolumeName)
		kubernetes.RemoveVolumeMountByName(container, PKCS11ConfigVolumeName)
		kubernetes.RemoveVolumeMountByName(container, PKCS11CertVolumeName)
		pkcs11helpers.CleanupHSMResources(&template.Spec, container)
		meta.RemoveStatusCondition(&instance.Status.Conditions, PKCS11Condition)

		container.Env = slices.DeleteFunc(container.Env, func(e core.EnvVar) bool {
			return e.Name == "PASSWORD"
		})

		container.Args = append(container.Args,
			"--ca=fileca",
			"--fileca-key",
			"/var/run/fulcio-secrets/key.pem",
			"--fileca-cert",
			"/var/run/fulcio-secrets/cert.pem",
		)

		if instance.Status.Certificate.PrivateKeyPasswordRef != nil {
			env := kubernetes.FindEnvByNameOrCreate(container, "PASSWORD")
			env.ValueFrom = &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					Key: instance.Status.Certificate.PrivateKeyPasswordRef.Key,
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyPasswordRef.Name,
					},
				},
			}
			container.Args = append(container.Args, "--fileca-key-passwd", "$(PASSWORD)")
		}

		ensureSignerVolumes(container, template, instance)

		cert := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-cert")
		cert.VolumeSource = core.VolumeSource{
			Projected: &core.ProjectedVolumeSource{
				DefaultMode: ptr.To(int32(0644)),
			},
		}
		cert.Projected.Sources = []core.VolumeProjection{
			{
				Secret: &core.SecretProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyRef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Certificate.PrivateKeyRef.Key,
							Path: "key.pem",
						},
					},
				},
			},
			{
				Secret: &core.SecretProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.CARef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Certificate.CARef.Key,
							Path: fulcioCertPemKey,
						},
					},
				},
			},
		}

		return nil
	}
}

func (i deployAction) ensureKMSCADeployment(instance *rhtasv1.Fulcio) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
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

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)
		template := &dp.Spec.Template

		// Clean up PKCS#11-specific volumes when switching to KMS mode
		kubernetes.RemoveVolumeByName(&template.Spec, PKCS11ConfigVolumeName)
		kubernetes.RemoveVolumeByName(&template.Spec, PKCS11CertVolumeName)
		kubernetes.RemoveVolumeMountByName(container, PKCS11ConfigVolumeName)
		kubernetes.RemoveVolumeMountByName(container, PKCS11CertVolumeName)
		pkcs11helpers.CleanupHSMResources(&template.Spec, container)
		meta.RemoveStatusCondition(&instance.Status.Conditions, PKCS11Condition)

		container.Args = append(container.Args,
			"--ca=kmsca",
			"--kms-resource", instance.Spec.Signer.Kms.KeyResource,
		)

		ensureSignerVolumes(container, template, instance)

		cert := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-cert")
		cert.VolumeSource = core.VolumeSource{
			Projected: &core.ProjectedVolumeSource{
				DefaultMode: ptr.To(int32(0644)),
			},
		}
		cert.Projected.Sources = []core.VolumeProjection{
			{
				Secret: &core.SecretProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.CARef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Certificate.CARef.Key,
							Path: fulcioCertPemKey,
						},
					},
				},
			},
		}

		container.Args = append(container.Args, "--kms-cert-chain-path", "/var/run/fulcio-secrets/cert.pem")

		return nil
	}
}

func (i deployAction) ensurePKCS11Deployment(instance *rhtasv1.Fulcio) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		if instance.Spec.Signer.PKCS11 == nil {
			return errors.New("PKCS#11 config not specified")
		}
		if instance.Status.ServerConfigRef == nil {
			return errors.New("server config ref is not specified")
		}
		if instance.Status.Certificate == nil || instance.Status.Certificate.CARef == nil {
			return errors.New("CA certificate reference not yet resolved")
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)
		template := &dp.Spec.Template

		// Clean up file-mode volumes that should not be present in PKCS#11 mode
		kubernetes.RemoveVolumeByName(&template.Spec, "fulcio-cert")
		kubernetes.RemoveVolumeMountByName(container, "fulcio-cert")

		pkcs11Cfg := instance.Spec.Signer.PKCS11
		configRef := pkcs11Cfg.ConfigRef
		if configRef == nil {
			return fmt.Errorf("PKCS#11 configRef not yet resolved")
		}

		container.Args = append(container.Args,
			"--ca=pkcs11ca",
			fmt.Sprintf("--pkcs11-config-path=%s/%s", PKCS11ConfigMountPath, configRef.Key),
			fmt.Sprintf("--aws-hsm-root-ca-path=%s/%s",
				PKCS11CertMountPath, instance.Status.Certificate.CARef.Key),
		)
		if pkcs11Cfg.KeyID != nil {
			container.Args = append(container.Args,
				fmt.Sprintf("--hsm-caroot-id=%d", *pkcs11Cfg.KeyID))
		}

		// PKCS#11-specific volume mounts
		pkcs11ConfigMount := kubernetes.FindVolumeMountByNameOrCreate(container, PKCS11ConfigVolumeName)
		pkcs11ConfigMount.MountPath = PKCS11ConfigMountPath
		pkcs11ConfigMount.ReadOnly = true

		pkcs11CertMount := kubernetes.FindVolumeMountByNameOrCreate(container, PKCS11CertVolumeName)
		pkcs11CertMount.MountPath = PKCS11CertMountPath
		pkcs11CertMount.ReadOnly = true

		pkcs11helpers.EnsureHSMResources(&template.Spec, container, instance.Spec.Volumes)

		configMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-config")
		configMount.MountPath = "/etc/fulcio-config"

		oidcInfoMount := kubernetes.FindVolumeMountByNameOrCreate(container, "oidc-info")
		oidcInfoMount.MountPath = "/var/run/fulcio"

		// PKCS#11-specific volumes
		pkcs11ConfigVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, PKCS11ConfigVolumeName)
		pkcs11ConfigVol.VolumeSource = core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  configRef.Name,
				DefaultMode: ptr.To(int32(0644)),
				Items: []core.KeyToPath{
					{Key: configRef.Key, Path: configRef.Key},
				},
			},
		}

		pkcs11CertVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, PKCS11CertVolumeName)
		pkcs11CertVol.VolumeSource = core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  instance.Status.Certificate.CARef.Name,
				DefaultMode: ptr.To(int32(0644)),
				Items: []core.KeyToPath{
					{Key: instance.Status.Certificate.CARef.Key, Path: instance.Status.Certificate.CARef.Key},
				},
			},
		}

		// Shared volumes (same as file mode)
		config := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-config")
		config.VolumeSource = core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.ServerConfigRef.Name,
				},
				DefaultMode: ptr.To(int32(0644)),
			},
		}

		oidcInfo := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "oidc-info")
		oidcInfo.VolumeSource = core.VolumeSource{
			Projected: &core.ProjectedVolumeSource{
				DefaultMode: ptr.To(int32(0644)),
				Sources: []core.VolumeProjection{{
					ConfigMap: &core.ConfigMapProjection{
						LocalObjectReference: core.LocalObjectReference{Name: "kube-root-ca.crt"},
						Items: []core.KeyToPath{
							{Key: kubeRootCACertKey, Path: kubeRootCACertKey, Mode: ptr.To(int32(0666))},
						},
					},
				}},
			},
		}

		return nil
	}
}
