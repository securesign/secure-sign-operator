package actions

import (
	"context"
	"fmt"
	"maps"
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
	"k8s.io/utils/ptr"
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

		isPKCS11 := instance.Spec.SignerType == rhtasv1.CTlogSignerTypePKCS11

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

		if isPKCS11 {
			p := instance.Spec.PKCS11
			if p == nil || instance.Status.PKCS11 == nil {
				return fmt.Errorf("PKCS#11 config not yet resolved — waiting for ensure-pkcs11-config")
			}
			// Add --pkcs11_module_path flag pointing to the copied .so in the shared volume
			modulePath := fmt.Sprintf("%s/%s", HSMLibMountPath, path.Base(p.PKCS11ModulePath))
			appArgs = append(appArgs, fmt.Sprintf("--pkcs11_module_path=%s", modulePath))
		}

		container.Args = appArgs
		if instance.Spec.MaxCertChainSize != nil {
			container.Args = append(container.Args, "--max_cert_chain_size", fmt.Sprintf("%d", *instance.Spec.MaxCertChainSize))
		}

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
		volumeMount.MountPath = "/ctfe-keys"
		volumeMount.ReadOnly = true

		if isPKCS11 {
			i.ensurePKCS11Deployment(instance, template, container)
		} else {
			// Clean up PKCS#11 resources that may remain from a previous pkcs11→file mode switch
			template.Spec.InitContainers = nil
			kubernetes.RemoveVolumeByName(&template.Spec, HSMTokensVolumeName)
			kubernetes.RemoveVolumeByName(&template.Spec, HSMLibVolumeName)
			kubernetes.RemoveVolumeMountByName(container, HSMTokensVolumeName)
			kubernetes.RemoveVolumeMountByName(container, HSMLibVolumeName)
		}

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

func (i deployAction) ensurePKCS11Deployment(instance *rhtasv1.CTlog, template *core.PodTemplateSpec, container *core.Container) {
	pkcs11Config := instance.Spec.PKCS11

	// Reconcile init containers in-place to preserve Kubernetes-defaulted fields
	// (TerminationMessagePath, TerminationMessagePolicy, ImagePullPolicy).
	reconcileInitContainers(&template.Spec, pkcs11Config.InitContainers, instance.Status.PKCS11.PinSecretRef, pkcs11Config.PKCS11ModulePath) //nolint:actionlint // template.Spec is the pod template, not the CR spec

	// Add serverEnv from PKCS#11 config
	for _, env := range pkcs11Config.ServerEnv {
		e := kubernetes.FindEnvByNameOrCreate(container, env.Name)
		e.Value = env.Value
		e.ValueFrom = env.ValueFrom
	}

	// Main container: HSM volume mounts
	hsmTokensMount := kubernetes.FindVolumeMountByNameOrCreate(container, HSMTokensVolumeName)
	hsmTokensMount.MountPath = HSMTokenMountPath

	hsmLibMount := kubernetes.FindVolumeMountByNameOrCreate(container, HSMLibVolumeName)
	hsmLibMount.MountPath = HSMLibMountPath
	hsmLibMount.ReadOnly = true

	// Add user-defined serverVolumeMounts
	for _, vm := range pkcs11Config.ServerVolumeMounts {
		m := kubernetes.FindVolumeMountByNameOrCreate(container, vm.Name)
		m.MountPath = vm.MountPath
		m.SubPath = vm.SubPath
		m.ReadOnly = vm.ReadOnly
	}

	// Process user-defined volumes first so operator-managed volumes take
	// precedence for reserved names (hsm-tokens, hsm-lib, etc.)
	for _, vol := range pkcs11Config.Volumes {
		v := kubernetes.FindVolumeByNameOrCreate(&template.Spec, vol.Name) //nolint:actionlint // template.Spec is the pod template, not the CR spec
		v.VolumeSource = vol.VolumeSource
		ensureVolumeDefaultMode(v)
	}

	// Operator-managed volumes: hsm-tokens defaults to EmptyDir but can be
	// overridden by user-defined volumes or PVC for token persistence.
	if !hasVolume(&template.Spec, HSMTokensVolumeName) {
		tokensVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, HSMTokensVolumeName) //nolint:actionlint // template.Spec is the pod template, not the CR spec
		// Clear previous VolumeSource before setting new one to prevent collision
		tokensVol.VolumeSource = core.VolumeSource{}
		if pkcs11Config.Persistence != nil && pkcs11Config.Persistence.Name != "" {
			tokensVol.PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pkcs11Config.Persistence.Name,
			}
		} else {
			tokensVol.EmptyDir = &core.EmptyDirVolumeSource{}
		}
	}

	if !hasVolume(&template.Spec, HSMLibVolumeName) {
		hsmLibVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, HSMLibVolumeName) //nolint:actionlint // template.Spec is the pod template, not the CR spec
		hsmLibVol.EmptyDir = &core.EmptyDirVolumeSource{}
	}
}

// reconcileInitContainers reconciles PKCS#11 init containers in-place on the
// pod spec. It uses FindInitContainerByNameOrCreate to modify existing containers
// rather than replacing the entire slice, which preserves Kubernetes API
// server-defaulted fields and avoids spurious diffs that would cause infinite
// reconciliation loops. It also manages the operator-owned hsm-lib-export
// container that copies the PKCS#11 .so to a shared volume.
func reconcileInitContainers(podSpec *core.PodSpec, specs []rhtasv1.PKCS11InitContainerSpec, pinSecretRef *rhtasv1.SecretKeySelector, pkcs11ModulePath string) {
	desiredNames := make(map[string]struct{}, len(specs)+1)
	for _, spec := range specs {
		desiredNames[spec.Name] = struct{}{}
		c := kubernetes.FindInitContainerByNameOrCreate(podSpec, spec.Name)
		c.Image = spec.Image
		c.Command = spec.Command
		c.Args = spec.Args
		c.EnvFrom = spec.EnvFrom
		if spec.Resources != nil {
			c.Resources = *spec.Resources
		}
		c.SecurityContext = spec.SecurityContext
		// Only set ImagePullPolicy if explicitly specified; otherwise preserve
		// the Kubernetes default applied by the API server.
		if spec.ImagePullPolicy != "" {
			c.ImagePullPolicy = spec.ImagePullPolicy
		}

		// Build env list: user-specified + injected HSM_PIN
		env := append([]core.EnvVar{}, spec.Env...)
		if pinSecretRef != nil {
			env = append(env, core.EnvVar{
				Name: HSMPinEnvVar,
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						Key:                  pinSecretRef.Key,
						LocalObjectReference: core.LocalObjectReference{Name: pinSecretRef.Name},
					},
				},
			})
		}
		c.Env = env

		// Build volume mounts: user-specified + operator-managed (skip duplicates by path)
		mounts := append([]core.VolumeMount{}, spec.VolumeMounts...)
		if !hasMountPath(mounts, HSMTokenMountPath) {
			mounts = append(mounts, core.VolumeMount{
				Name:      HSMTokensVolumeName,
				MountPath: HSMTokenMountPath,
			})
		}
		if !hasMountPath(mounts, HSMLibMountPath) {
			mounts = append(mounts, core.VolumeMount{
				Name:      HSMLibVolumeName,
				MountPath: HSMLibMountPath,
			})
		}
		c.VolumeMounts = mounts
	}

	// Operator-managed hsm-lib-export container: copies the PKCS#11 .so
	// from the vendor image to the shared lib volume.
	if pkcs11ModulePath != "" && len(specs) > 0 {
		desiredNames[HSMLibExportContainerName] = struct{}{}
		libExport := kubernetes.FindInitContainerByNameOrCreate(podSpec, HSMLibExportContainerName)
		libExport.Image = specs[0].Image
		libExport.Command = []string{"cp", pkcs11ModulePath, fmt.Sprintf("%s/", HSMLibMountPath)}
		libExport.VolumeMounts = []core.VolumeMount{
			{Name: HSMLibVolumeName, MountPath: HSMLibMountPath},
		}
	}

	// Remove init containers that are no longer in the spec
	if len(desiredNames) == 0 {
		podSpec.InitContainers = nil
		return
	}
	filtered := make([]core.Container, 0, len(podSpec.InitContainers))
	for _, c := range podSpec.InitContainers {
		if _, ok := desiredNames[c.Name]; ok {
			filtered = append(filtered, c)
		}
	}
	podSpec.InitContainers = filtered
}

// hasVolume returns true if the pod spec already contains a volume with the given name.
func hasVolume(podSpec *core.PodSpec, name string) bool {
	for _, v := range podSpec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

// hasMountPath returns true if any mount in the slice uses the given path.
func hasMountPath(mounts []core.VolumeMount, path string) bool {
	for _, m := range mounts {
		if m.MountPath == path {
			return true
		}
	}
	return false
}

// ensureVolumeDefaultMode applies the same DefaultMode that the Kubernetes API
// server would apply (0644 / octal 0644 = 420 decimal) to volume sources that
// support it. Without this, a user-specified volume that omits DefaultMode
// would differ from the API server's response on every reconcile (nil vs *420),
// causing an infinite update loop.
func ensureVolumeDefaultMode(v *core.Volume) {
	defaultMode := ptr.To(int32(0644))
	if v.ConfigMap != nil && v.ConfigMap.DefaultMode == nil {
		v.ConfigMap.DefaultMode = defaultMode
	}
	if v.Secret != nil && v.Secret.DefaultMode == nil {
		v.Secret.DefaultMode = defaultMode
	}
	if v.Projected != nil && v.Projected.DefaultMode == nil {
		v.Projected.DefaultMode = defaultMode
	}
	if v.DownwardAPI != nil && v.DownwardAPI.DefaultMode == nil {
		v.DownwardAPI.DefaultMode = defaultMode
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
