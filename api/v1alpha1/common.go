package v1alpha1

import v1 "k8s.io/api/core/v1"

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	Enabled bool `json:"enabled,omitempty"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
}

type MonitoringConfig struct {
	// If true, the Operator will create monitoring resources
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

// PVCStruct
type Pvc struct {
	// Size of the PVC
	//+kubebuilder:default:="5Gi"
	Size string `json:"size,omitempty"`
	// Retain policy for the PVC
	//+kubebuilder:default:=true
	Retain bool `json:"retain"`
	// Name of the PVC
	Name string `json:"name,omitempty"`
	// Storage class for the PVC
	StorageClass string `json:"storageClass,omitempty"`
}
