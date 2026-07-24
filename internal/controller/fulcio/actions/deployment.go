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
	"github.com/securesign/operator/internal/utils/tls"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1 "github.com/securesign/operator/api/v1"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/images"
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
		// need to add Fulcio's unix domain socket used for the legacy gRPC server other way it will be
		// rest v1 api will be routed through proxy
		deployment.Proxy("@fulcio-legacy-grpc-socket"),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), containerName),
		deployment.PodRequirements(instance.Spec.PodRequirements, containerName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance)
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

func (i deployAction) resolveCTlogUrl(instance *rhtasv1.Fulcio) (string, error) {
	if instance.Spec.Ctlog.Prefix == "" {
		return "", futils.ErrCtlogPrefixNotSpecified
	}

	if instance.Spec.Ctlog.Address != "" {
		url := instance.Spec.Ctlog.Address
		if instance.Spec.Ctlog.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Ctlog.Port)
		}
		return fmt.Sprintf("%s/%s", url, instance.Spec.Ctlog.Prefix), nil
	}

	var (
		protocol string
	)
	if tls.UseTlsClient(instance) {
		protocol = "https"
	} else {
		protocol = "http"
	}
	return fmt.Sprintf("%s://ctlog.%s.svc/%s", protocol, instance.Namespace, instance.Spec.Ctlog.Prefix), nil
}

func (i deployAction) ensureDeployment(instance *rhtasv1.Fulcio, sa string, labels map[string]string) func(deployment *v1.Deployment) error {
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

		if instance.Spec.Certificate.CAType == rhtasv1.CATypePKCS11 {
			return i.ensurePKCS11Deployment(instance, sa, labels, dp)
		}
		return i.ensureFileCADeployment(instance, sa, labels, dp)
	}
}

func (i deployAction) ensureFileCADeployment(instance *rhtasv1.Fulcio, sa string, labels map[string]string, dp *v1.Deployment) error {
	if instance.Status.Certificate.PrivateKeyRef == nil {
		return errors.New("private key secret is not specified")
	}

	ctlogUrl, err := i.resolveCTlogUrl(instance)
	if err != nil {
		return fmt.Errorf("could not resolve CTLog url: %w", err)
	}

	args := []string{
		"serve",
		"--port=5555",
		"--grpc-port=5554",
		fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
		"--ca=fileca",
		"--fileca-key",
		"/var/run/fulcio-secrets/key.pem",
		"--fileca-cert",
		"/var/run/fulcio-secrets/cert.pem",
		fmt.Sprintf("--ct-log-url=%s", ctlogUrl),
	}

	spec := &dp.Spec
	spec.Replicas = utils.Pointer[int32](1)
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	template := &spec.Template
	template.Labels = labels
	template.Spec.ServiceAccountName = sa
	template.Spec.AutomountServiceAccountToken = &[]bool{true}[0]

	// Clean up PKCS#11 resources that may remain from a previous pkcs11→file mode switch
	template.Spec.InitContainers = nil
	kubernetes.RemoveVolumeByName(&template.Spec, HSMTokensVolumeName)
	kubernetes.RemoveVolumeByName(&template.Spec, HSMLibVolumeName)
	kubernetes.RemoveVolumeByName(&template.Spec, PKCS11ConfigVolumeName)
	kubernetes.RemoveVolumeByName(&template.Spec, PKCS11CertVolumeName)

	container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)

	// Remove stale PKCS#11 volume mounts from the main container
	kubernetes.RemoveVolumeMountByName(container, HSMTokensVolumeName)
	kubernetes.RemoveVolumeMountByName(container, HSMLibVolumeName)
	kubernetes.RemoveVolumeMountByName(container, PKCS11ConfigVolumeName)
	kubernetes.RemoveVolumeMountByName(container, PKCS11CertVolumeName)
	container.Image = images.Registry.Get(images.FulcioServer)

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
		args = append(args, "--fileca-key-passwd", "$(PASSWORD)")
	}

	if fips.Enabled() {
		args = append(args, "--client-signing-algorithms", fips.ClientSigningAlgorithms)
	}

	container.Args = args

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

	certMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-cert")
	certMount.MountPath = "/var/run/fulcio-secrets"
	certMount.ReadOnly = true

	configMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-config")
	configMount.MountPath = "/etc/fulcio-config"

	oidcInfoMount := kubernetes.FindVolumeMountByNameOrCreate(container, "oidc-info")
	oidcInfoMount.MountPath = "/var/run/fulcio"

	config := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-config")
	if config.ConfigMap == nil {
		config.ConfigMap = &core.ConfigMapVolumeSource{}
	}
	config.ConfigMap.Name = instance.Status.ServerConfigRef.Name

	cert := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-cert")
	if cert.Projected == nil {
		cert.Projected = &core.ProjectedVolumeSource{}
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
						Path: CertPEMKey,
					},
				},
			},
		},
	}

	oidcInfo := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "oidc-info")
	if oidcInfo.Projected == nil {
		oidcInfo.Projected = &core.ProjectedVolumeSource{}
	}
	oidcInfo.Projected.Sources = []core.VolumeProjection{
		{
			ConfigMap: &core.ConfigMapProjection{
				LocalObjectReference: core.LocalObjectReference{
					Name: "kube-root-ca.crt",
				},
				Items: []core.KeyToPath{
					{
						Key:  CACrtKey,
						Path: CACrtKey,
						Mode: ptr.To(int32(0666)),
					},
				},
			},
		},
	}

	i.setProbes(container)
	return nil
}

func (i deployAction) ensurePKCS11Deployment(instance *rhtasv1.Fulcio, sa string, labels map[string]string, dp *v1.Deployment) error {
	pkcs11Config := instance.Spec.Certificate.PKCS11
	if pkcs11Config == nil {
		return errors.New("pkcs11 config is not specified")
	}
	if instance.Status.PKCS11 == nil || instance.Status.PKCS11.PKCS11ConfigRef == nil || instance.Status.PKCS11.CredentialsRef == nil {
		return errors.New("pkcs11 status is not fully populated")
	}

	ctlogUrl, err := i.resolveCTlogUrl(instance)
	if err != nil {
		return fmt.Errorf("could not resolve CTLog url: %w", err)
	}

	args := []string{
		"serve",
		"--port=5555",
		"--grpc-port=5554",
		fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
		"--ca=pkcs11ca",
		fmt.Sprintf("--pkcs11-config-path=%s/%s", PKCS11ConfigMountPath, instance.Status.PKCS11.PKCS11ConfigRef.Key),
		fmt.Sprintf("--hsm-caroot-id=%d", pkcs11Config.KeyConfig.ID),
		fmt.Sprintf("--aws-hsm-root-ca-path=%s/%s", PKCS11CertMountPath, instance.Status.Certificate.CARef.Key),
		fmt.Sprintf("--ct-log-url=%s", ctlogUrl),
	}

	if fips.Enabled() {
		args = append(args, "--client-signing-algorithms", fips.ClientSigningAlgorithms)
	}

	spec := &dp.Spec
	spec.Replicas = utils.Pointer[int32](1)
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	template := &spec.Template
	template.Labels = labels
	template.Spec.ServiceAccountName = sa
	template.Spec.AutomountServiceAccountToken = &[]bool{true}[0]

	// Reconcile init containers in-place to preserve Kubernetes-defaulted fields
	// (TerminationMessagePath, TerminationMessagePolicy, ImagePullPolicy).
	// Replacing the entire slice with new Container objects would cause
	// CreateOrUpdate to detect a diff on every reconcile, resetting the state
	// to Creating and preventing the transition to Ready.
	reconcileInitContainers(&template.Spec, pkcs11Config.InitContainers, instance.Status.PKCS11.CredentialsRef)

	// Main container
	container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
	container.Image = images.Registry.Get(images.FulcioServer)
	container.Args = args

	// Add serverEnv from PKCS#11 config
	for _, env := range pkcs11Config.ServerEnv {
		e := kubernetes.FindEnvByNameOrCreate(container, env.Name)
		e.Value = env.Value
		e.ValueFrom = env.ValueFrom
	}

	// Ports
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

	// Volume mounts for main container
	hsmTokensMount := kubernetes.FindVolumeMountByNameOrCreate(container, HSMTokensVolumeName)
	hsmTokensMount.MountPath = HSMTokensMountPath

	hsmLibMount := kubernetes.FindVolumeMountByNameOrCreate(container, HSMLibVolumeName)
	hsmLibMount.MountPath = HSMLibMountPath

	pkcs11ConfigMount := kubernetes.FindVolumeMountByNameOrCreate(container, PKCS11ConfigVolumeName)
	pkcs11ConfigMount.MountPath = PKCS11ConfigMountPath
	pkcs11ConfigMount.ReadOnly = true

	pkcs11CertMount := kubernetes.FindVolumeMountByNameOrCreate(container, PKCS11CertVolumeName)
	pkcs11CertMount.MountPath = PKCS11CertMountPath
	pkcs11CertMount.ReadOnly = true

	configMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-config")
	configMount.MountPath = "/etc/fulcio-config"

	oidcInfoMount := kubernetes.FindVolumeMountByNameOrCreate(container, "oidc-info")
	oidcInfoMount.MountPath = "/var/run/fulcio"

	// Add user-defined serverVolumeMounts
	for _, vm := range pkcs11Config.ServerVolumeMounts {
		m := kubernetes.FindVolumeMountByNameOrCreate(container, vm.Name)
		m.MountPath = vm.MountPath
		m.SubPath = vm.SubPath
		m.ReadOnly = vm.ReadOnly
	}

	// Process user-defined volumes first so operator-managed volumes take
	// precedence for reserved names (pkcs11-config, fulcio-pkcs11-cert, etc.)
	for _, vol := range pkcs11Config.Volumes {
		v := kubernetes.FindVolumeByNameOrCreate(&template.Spec, vol.Name)
		v.VolumeSource = vol.VolumeSource
		// The Kubernetes API server defaults DefaultMode to 0644 for ConfigMap,
		// Secret, DownwardAPI, and Projected volume sources. If the user's spec
		// omits DefaultMode, applying the same default here avoids a spurious
		// diff on every reconcile (nil vs *420) that would cause CreateOrUpdate
		// to update the deployment, resetting the status to "Creating" and
		// preventing the transition to "Ready".
		ensureVolumeDefaultMode(v)
	}

	// Operator-managed volumes: hsm-tokens and hsm-lib default to EmptyDir but
	// can be overridden by user-defined volumes (e.g., PVC for token persistence,
	// or omitted entirely if the custom Fulcio image bundles the vendor SDK).
	if !hasVolume(&template.Spec, HSMTokensVolumeName) {
		hsmTokensVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, HSMTokensVolumeName)
		hsmTokensVol.EmptyDir = &core.EmptyDirVolumeSource{}
	}

	if !hasVolume(&template.Spec, HSMLibVolumeName) {
		hsmLibVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, HSMLibVolumeName)
		hsmLibVol.EmptyDir = &core.EmptyDirVolumeSource{}
	}

	// Operator-managed volumes that always override user definitions.
	// Use ensureVolumeDefaultMode after setting VolumeSource to prevent
	// the infinite reconciliation loop caused by nil DefaultMode vs API server default (420).
	pkcs11ConfigVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, PKCS11ConfigVolumeName)
	pkcs11ConfigVol.VolumeSource = core.VolumeSource{
		Secret: &core.SecretVolumeSource{
			SecretName: instance.Status.PKCS11.PKCS11ConfigRef.Name,
		},
	}
	ensureVolumeDefaultMode(pkcs11ConfigVol)

	pkcs11CertVol := kubernetes.FindVolumeByNameOrCreate(&template.Spec, PKCS11CertVolumeName)
	pkcs11CertVol.VolumeSource = core.VolumeSource{
		Secret: &core.SecretVolumeSource{
			SecretName: instance.Status.Certificate.CARef.Name,
		},
	}
	ensureVolumeDefaultMode(pkcs11CertVol)

	fulcioConfig := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-config")
	fulcioConfig.VolumeSource = core.VolumeSource{
		ConfigMap: &core.ConfigMapVolumeSource{
			LocalObjectReference: core.LocalObjectReference{
				Name: instance.Status.ServerConfigRef.Name,
			},
		},
	}
	ensureVolumeDefaultMode(fulcioConfig)

	oidcInfo := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "oidc-info")
	oidcInfo.VolumeSource = core.VolumeSource{
		Projected: &core.ProjectedVolumeSource{
			Sources: []core.VolumeProjection{
				{
					ConfigMap: &core.ConfigMapProjection{
						LocalObjectReference: core.LocalObjectReference{
							Name: "kube-root-ca.crt",
						},
						Items: []core.KeyToPath{
							{
								Key:  CACrtKey,
								Path: CACrtKey,
								Mode: ptr.To(int32(0666)),
							},
						},
					},
				},
			},
		},
	}
	ensureVolumeDefaultMode(oidcInfo)

	i.setProbes(container)
	return nil
}

// reconcileInitContainers reconciles PKCS#11 init containers in-place on the
// pod spec. It uses FindInitContainerByNameOrCreate to modify existing containers
// rather than replacing the entire slice, which preserves Kubernetes API
// server-defaulted fields (TerminationMessagePath, TerminationMessagePolicy,
// ImagePullPolicy) and avoids spurious diffs that would cause infinite
// reconciliation loops.
func reconcileInitContainers(podSpec *core.PodSpec, specs []rhtasv1.PKCS11InitContainerSpec, credentialsRef *rhtasv1.SecretKeySelector) {
	desiredNames := make(map[string]struct{}, len(specs))
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
		if credentialsRef != nil {
			env = append(env, core.EnvVar{
				Name: HSMPinEnvVar,
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						Key: credentialsRef.Key,
						LocalObjectReference: core.LocalObjectReference{
							Name: credentialsRef.Name,
						},
					},
				},
			})
		}
		c.Env = env

		// Build volume mounts: user-specified + operator-managed (skip duplicates by path)
		mounts := append([]core.VolumeMount{}, spec.VolumeMounts...)
		if !hasMountPath(mounts, HSMTokensMountPath) {
			mounts = append(mounts, core.VolumeMount{
				Name:      HSMTokensVolumeName,
				MountPath: HSMTokensMountPath,
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

func (i deployAction) setProbes(container *core.Container) {
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
}
