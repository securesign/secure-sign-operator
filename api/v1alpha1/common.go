package v1alpha1

import (
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
}

type MonitoringConfig struct {
	// If true, the Operator will create monitoring resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Enabled bool `json:"enabled"`
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

// Pvc configuration of the persistent storage claim for deployment in the cluster.
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
}
