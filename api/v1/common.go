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
	Tuf TufService `json:"tuf,omitempty"`
}

// TrillianService configuration to connect Trillian server
type TrillianService struct {
	// Address to Trillian Log Server End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Trillian Log Server End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// TufService configuration to connect TUF server
type TufService struct {
	// Address to TUF Server End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of TUF Server End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// CtlogService configuration to connect Ctlog server
type CtlogService struct {
	// Address to Ctlog Log Server End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Ctlog Log Server End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
	// Prefix is the name of the log. The prefix cannot be empty and can
	// contain "/" path separator characters to define global override handler prefix.
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9/]*[a-z0-9])?$"
	//+optional
	Prefix string `json:"prefix,omitempty"`
}

// FulcioService configuration to connect Fulcio server
type FulcioService struct {
	// Address to Fulcio End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Fulcio End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// RekorService configuration to connect Rekor server
type RekorService struct {
	// Address to Rekor End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of Rekor End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// TsaService configuration to connect TSA server
type TsaService struct {
	// Address is the full TSA endpoint URL including the API suffix path
	// (e.g. http://tsa.example.com/api/v1/timestamp).
	//+optional
	Address string `json:"address,omitempty"`
	// Port of TSA End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
}

// ServiceReference identifies a component service either by in-cluster CR
// reference or by an external URL.
// +kubebuilder:validation:XValidation:rule="!(has(self.ref) && has(self.url) && size(self.url) > 0)",message="ref and url are mutually exclusive"
type ServiceReference struct {
	// In-cluster reference to a component CR.
	//+optional
	Ref *ServiceReferenceRef `json:"ref,omitempty"`
	// Direct URL for an external or cross-namespace service.
	//+optional
	URL string `json:"url,omitempty"`
}

// ServiceReferenceRef identifies a component CR by name and optional namespace.
type ServiceReferenceRef struct {
	// Name of the referenced CR.
	//+required
	//+kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the referenced CR. Defaults to the namespace of the referencing resource.
	//+optional
	Namespace string `json:"namespace,omitempty"`
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

type PodRequirements struct {
	// Number of desired pods.
	// +optional
	// +kubebuilder:validation:Minimum:=0
	Replicas    *int32                     `json:"replicas,omitempty"`
	Affinity    *core.Affinity             `json:"affinity,omitempty"`
	Resources   *core.ResourceRequirements `json:"resources,omitempty"`
	Tolerations []core.Toleration          `json:"tolerations,omitempty"`
}
