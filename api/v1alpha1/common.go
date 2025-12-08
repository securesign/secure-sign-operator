package v1alpha1

import (
	core "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
	// Set Route Selector Labels for ingress sharding.
	//+kubebuilder:validation:XValidation:rule="(oldSelf.size() == 0 || self == oldSelf)",message=RouteSelectorLabels can't be modified
	RouteSelectorLabels map[string]string `json:"routeSelectorLabels,omitempty"`
}

// TlogMonitoring configures monitoring for the Rekor transparency log.
type TlogMonitoring struct {
	// If true, the Operator will create the Rekor log monitor resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// Interval between log monitoring checks
	//+kubebuilder:default:="10m"
	//+optional
	Interval metav1.Duration `json:"interval"`
}
type MonitoringConfig struct {
	// If true, the Operator will create monitoring resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled bool `json:"enabled"`
}

type MonitoringWithTLogConfig struct {
	// Base monitoring configuration
	MonitoringConfig `json:",inline"`
	// Configuration for Rekor transparency log monitoring
	//+optional
	TLog TlogMonitoring `json:"tlog"`
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
	//+kubebuilder:default:=8091
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
	//+kubebuilder:default:=trusted-artifact-signer
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
	// Address to TSA End point
	//+optional
	Address string `json:"address,omitempty"`
	// Port of TSA End point
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	//+optional
	Port *int32 `json:"port,omitempty"`
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
	//+kubebuilder:default:="5Gi"
	Size *k8sresource.Quantity `json:"size,omitempty"`

	// Retain policy for the PVC
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Retain *bool `json:"retain"`
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
	//+kubebuilder:default:={ReadWriteOnce}
	//+kubebuilder:validation:MinItems:=1
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

type Auth struct {
	// Environmental variables used to define authentication parameters
	//+optional
	Env []core.EnvVar `json:"env,omitempty"`
	// Secret ref to be mounted inside a pod, Mount path defaults to /var/run/secrets/tas/auth
	//+optional
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

type PodRequirements struct {
	// Number of desired pods.
	// +optional
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:default:=1
	Replicas    *int32                     `json:"replicas,omitempty"`
	Affinity    *core.Affinity             `json:"affinity,omitempty"`
	Resources   *core.ResourceRequirements `json:"resources,omitempty"`
	Tolerations []core.Toleration          `json:"tolerations,omitempty"`
}
