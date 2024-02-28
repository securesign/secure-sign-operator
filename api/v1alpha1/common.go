package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	Enabled bool `json:"enabled,omitempty"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
}

type MonitoringConfig struct {
	// If true, the Operator will create monitoring resources
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	Enabled bool `json:"enabled,omitempty"`
}

// SecretKeySelector selects a key of a Secret.
// +structType=atomic
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	v1.LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from.  Must be a valid secret key.
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
