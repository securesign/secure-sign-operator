package v1

import (
	core "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Ingress struct {
	// If set to true, the Operator will create a Kubernetes Ingress resource.
	// On OpenShift, the platform automatically derives a Route from this Ingress, using "edge" TLS termination by default.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	Enabled *bool `json:"enabled,omitempty"`
	// Set hostname for your Ingress.
	Host string `json:"host,omitempty"`
	// Set labels applied to the created Ingress, e.g. for ingress-controller/route selection when sharding ingress traffic.
	//+kubebuilder:validation:XValidation:rule="(oldSelf.size() == 0 || self == oldSelf)",message=Labels can't be modified
	Labels map[string]string `json:"labels,omitempty"`
}

// TlogMonitoring configures monitoring for the Rekor transparency log.
type TlogMonitoring struct {
	// If true, the Operator will create the Rekor log monitor resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	Enabled *bool `json:"enabled,omitempty"`
	// Interval between log monitoring checks.
	// Minimum interval is 10 seconds to avoid excessive load on the log server.
	//+kubebuilder:validation:XValidation:rule="duration(self) >= duration('10s')",message=Interval must be at least 10 seconds
	//+optional
	Interval *metav1.Duration `json:"interval,omitempty"`
}

// MonitoringConfig configures observability for the component.
// +kubebuilder:validation:XValidation:rule="!has(self.serviceMonitor) || !has(self.serviceMonitor.enabled) || !self.serviceMonitor.enabled || (has(self.metrics) && has(self.metrics.enabled) && self.metrics.enabled)",message="ServiceMonitor requires metrics to be enabled"
type MonitoringConfig struct {
	// Metrics endpoint configuration.
	// Controls whether the operator exposes a metrics HTTP endpoint
	// on the component's pods and services.
	// +optional
	Metrics MetricsConfig `json:"metrics,omitempty"`

	// Prometheus ServiceMonitor configuration.
	// Controls whether the operator creates ServiceMonitor resources
	// for automated metrics discovery and scraping.
	// Requires metrics to be enabled.
	// +optional
	ServiceMonitor ServiceMonitorConfig `json:"serviceMonitor,omitempty"`
}

// MetricsConfig configures the metrics endpoint exposed by component
// pods and services.
type MetricsConfig struct {
	// Enable metrics endpoint on the component's pods and services.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ServiceMonitorConfig configures the creation of Prometheus
// ServiceMonitor resources for automated metrics discovery.
type ServiceMonitorConfig struct {
	// Enable creation of ServiceMonitor resources.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

type MonitoringWithTLogConfig struct {
	// Base monitoring configuration
	MonitoringConfig `json:",inline"`
	// Configuration for Rekor transparency log monitoring
	//+optional
	TLog TlogMonitoring `json:"tlog,omitempty"`
	// TUF service configuration
	//+optional
	Tuf ServiceReference `json:"tuf,omitempty"`
}

// ServiceReference identifies a component service either by in-cluster CR
// reference or by an external URL.
// +kubebuilder:validation:XValidation:rule="!(has(self.ref) && has(self.url) && size(self.url) > 0)",message="ref and url are mutually exclusive"
type ServiceReference struct {
	// In-cluster reference to a component CR.
	//+optional
	Ref *ServiceReferenceRef `json:"ref,omitempty"`
	// Direct URL for an external or cross-namespace service.
	// Accepts: host:port, dns:///host:port, http(s)://host/path
	//+optional
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=2048
	URL string `json:"url,omitempty"`
}

func (s ServiceReference) GetServiceRef() ServiceReference { return s }

// ServiceReferenceRef identifies a component CR by name and namespace.
type ServiceReferenceRef struct {
	// Name of the referenced CR.
	//+required
	//+kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the referenced CR.
	//+required
	//+kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
// +structType=atomic
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
}

// SecretKeySelector selects a key of a Secret.
// +structType=atomic
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from. Must be a valid secret key.
	//+required
	//+kubebuilder:validation:Pattern:="^[-._a-zA-Z0-9]+$"
	Key string `json:"key" protobuf:"bytes,2,opt,name=key"`
}

// +kubebuilder:validation:Enum:=ReadWriteOnce;ReadOnlyMany;ReadWriteMany;ReadWriteOncePod
type PersistentVolumeAccessMode core.PersistentVolumeAccessMode

// Pvc configuration of the persistent storage claim for deployment in the cluster.
// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.storageClass) || has(self.storageClass) && oldSelf.storageClass == self.storageClass)",message="storageClass is immutable when a PVC name is not specified"
// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.accessModes) || has(self.accessModes) && oldSelf.accessModes == self.accessModes)",message="accessModes is immutable when a PVC name is not specified"
type Pvc struct {
	// The requested size of the persistent volume attached to Pod.
	// The format of this field matches that defined by kubernetes/apimachinery.
	// See https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity for more info on the format of this field.
	Size *k8sresource.Quantity `json:"size,omitempty"`

	// Retain policy for the PVC
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Retain *bool `json:"retain,omitempty"`
	// Name of the PVC
	//+optional
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`
	// The name of the StorageClass to claim a PersistentVolume from.
	//+optional
	StorageClass string `json:"storageClass,omitempty"`
	// PVC AccessModes
	//+kubebuilder:validation:MinItems:=1
	// +listType=set
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// KMS configures a remote key management service for signing operations.
// +kubebuilder:validation:XValidation:rule="self.keyResource.matches('^(gcpkms|azurekms|hashivault|openbao|awskms)://.+$')",message="keyResource must be a valid KMS URI (gcpkms://, azurekms://, hashivault://, openbao://, or awskms://)"
type KMS struct {
	// KMS key resource URI. Valid schemes: gcpkms://, azurekms://, hashivault://, openbao://, awskms://
	//+required
	KeyResource string `json:"keyResource"`
}

type Auth struct {
	// Environmental variables used to define authentication parameters
	//+optional
	// +listType=map
	// +listMapKey=name
	Env []core.EnvVar `json:"env,omitempty"`
	// Secret ref to be mounted inside a pod, Mount path defaults to /var/run/secrets/tas/auth
	//+optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=key
	SecretMount []SecretKeySelector `json:"secretMount,omitempty"`
}

// TLS (Transport Layer Security) Configuration for enabling service encryption.
// +kubebuilder:validation:XValidation:rule=(!has(self.certificateRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type TLS struct {
	// Reference to the private key secret used for TLS encryption.
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to the certificate secret used for TLS encryption.
	//+optional
	CertRef *SecretKeySelector `json:"certificateRef,omitempty"`
}

// ServiceAccountConfig configures the component's ServiceAccount.
type ServiceAccountConfig struct {
	// ImagePullSecrets is an optional list of references to secrets in the same namespace
	// to use for pulling container images used by this component.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []core.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// InitContainerSpec defines a curated subset of corev1.Container for custom init containers.
// These containers run before the main server to perform vendor-specific initialization.
type InitContainerSpec struct {
	// Name of the init container. Must be unique within the pod.
	//+required
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	Name string `json:"name"`
	// Container image name.
	//+required
	//+kubebuilder:validation:MinLength=1
	Image string `json:"image"`
	// Entrypoint array. Not executed within a shell.
	//+optional
	Command []string `json:"command,omitempty"`
	// Arguments to the entrypoint.
	//+optional
	Args []string `json:"args,omitempty"`
	// List of environment variables to set in the container.
	//+optional
	// +listType=map
	// +listMapKey=name
	Env []core.EnvVar `json:"env,omitempty"`
	// List of sources to populate environment variables in the container.
	//+optional
	EnvFrom []core.EnvFromSource `json:"envFrom,omitempty"`
	// Pod volumes to mount into the container's filesystem.
	//+optional
	// +listType=map
	// +listMapKey=name
	VolumeMounts []core.VolumeMount `json:"volumeMounts,omitempty"`
	// Compute Resources required by this container.
	//+optional
	Resources *core.ResourceRequirements `json:"resources,omitempty"`
	// SecurityContext defines the security options the container should be run with.
	//+optional
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty"`
	// Image pull policy.
	//+optional
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Restart policy for the init container. Set to "Always" to create a
	// native sidecar container (Kubernetes 1.29+) that starts before the
	// main container and runs for the pod's lifetime.
	// +optional
	// +kubebuilder:validation:Enum=Always
	RestartPolicy *core.ContainerRestartPolicy `json:"restartPolicy,omitempty"`
}

// PodExtensions groups user-specified pod customization fields: init containers,
// volumes, and volume mounts. Embed with json:",inline" to keep the JSON paths
// at the spec level (e.g. spec.initContainers, not spec.podExtensions.initContainers).
type PodExtensions struct {
	// InitContainers to run before the main server container.
	//+optional
	// +listType=map
	// +listMapKey=name
	InitContainers []InitContainerSpec `json:"initContainers,omitempty"`
	// Additional volumes to attach to the deployment pods.
	// Only a curated set of volume source types is permitted.
	//+optional
	// +listType=map
	// +listMapKey=name
	Volumes []AdditionalVolume `json:"volumes,omitempty"`
	// Additional volume mounts for the main server container.
	//+optional
	// +listType=map
	// +listMapKey=name
	VolumeMounts []core.VolumeMount `json:"volumeMounts,omitempty"`
}

// AdditionalVolume defines a named volume with a restricted set of source types.
// This avoids exposing the full corev1.VolumeSource (30+ types) in the CRD schema.
// MaxProperties=2 enforces exactly one volume source: the merged object has
// "name" + one source field = 2 properties. Setting two sources (e.g. secret +
// configMap) would produce 3 properties and be rejected.
// +kubebuilder:validation:MinProperties=2
// +kubebuilder:validation:MaxProperties=2
type AdditionalVolume struct {
	// Name of the volume. Must be unique within the pod.
	//+required
	//+kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Source of the volume.
	AdditionalVolumeSource `json:",inline"`
}

// AdditionalVolumeSource restricts volume sources to the types commonly needed
// for operator extensions: Secret, ConfigMap, EmptyDir, PVC, CSI, and Projected.
type AdditionalVolumeSource struct {
	// Secret represents a secret that should populate this volume.
	//+optional
	Secret *core.SecretVolumeSource `json:"secret,omitempty"`
	// ConfigMap represents a configMap that should populate this volume.
	//+optional
	ConfigMap *core.ConfigMapVolumeSource `json:"configMap,omitempty"`
	// EmptyDir represents a temporary directory that shares a pod's lifetime.
	//+optional
	EmptyDir *core.EmptyDirVolumeSource `json:"emptyDir,omitempty"`
	// PersistentVolumeClaim represents a reference to a PersistentVolumeClaim.
	//+optional
	PersistentVolumeClaim *core.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
	// CSI represents ephemeral storage provided by external CSI drivers.
	//+optional
	CSI *core.CSIVolumeSource `json:"csi,omitempty"`
	// Projected items for all in one resources secrets, configmaps, and downward API.
	//+optional
	Projected *core.ProjectedVolumeSource `json:"projected,omitempty"`
}

// ToVolume converts an AdditionalVolume to a core.Volume.
func (v *AdditionalVolume) ToVolume() core.Volume {
	return core.Volume{
		Name: v.Name,
		VolumeSource: core.VolumeSource{
			Secret:                v.Secret,
			ConfigMap:             v.ConfigMap,
			EmptyDir:              v.EmptyDir,
			PersistentVolumeClaim: v.PersistentVolumeClaim,
			CSI:                   v.CSI,
			Projected:             v.Projected,
		},
	}
}

type PodRequirements struct {
	// Number of desired pods.
	// +optional
	// +kubebuilder:validation:Minimum:=0
	Replicas    *int32                     `json:"replicas,omitempty"`
	Affinity    *core.Affinity             `json:"affinity,omitempty"`
	Resources   *core.ResourceRequirements `json:"resources,omitempty"`
	Tolerations []core.Toleration          `json:"tolerations,omitempty"`
}

// PKCS11Config configures connection to a PKCS#11 HSM module.
type PKCS11Config struct {
	// Absolute path to the PKCS#11 module (.so).
	//+optional
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:Pattern=`^/.+\..+$`
	ModulePath string `json:"modulePath,omitempty"`
	// Token label identifying the HSM slot.
	//+optional
	//+kubebuilder:validation:MinLength=1
	TokenLabel string `json:"tokenLabel,omitempty"`
	// Numeric slot ID (alternative to tokenLabel).
	//+optional
	//+kubebuilder:validation:Minimum=0
	SlotNumber *int32 `json:"slotNumber,omitempty"`
	// Reference to a Secret key containing the HSM user PIN.
	//+optional
	PinSecretRef *SecretKeySelector `json:"pinSecretRef,omitempty"`
	// PKCS#11 CKA_ID of the signing key.
	//+optional
	//+kubebuilder:validation:Minimum=0
	KeyID *int32 `json:"keyID,omitempty"`
	// PKCS#11 CKA_LABEL of the signing key.
	//+optional
	//+kubebuilder:validation:MinLength=1
	KeyLabel string `json:"keyLabel,omitempty"`
}
