package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RekorSpec defines the desired state of Rekor
type RekorSpec struct {
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	// Persistent volume claim name to bound with Rekor component
	PvcName string `json:"pvcName,omitempty"`
	//Enable Service monitors for rekor
	Monitoring bool `json:"monitoring,omitempty"`
	//Rekor Search UI
	RekorSearchUI RekorSearchUI `json:"rekorSearchUI,omitempty"`
	// Signer configuration
	Signer RekorSigner `json:"signer,omitempty"`
}

type RekorSigner struct {
	// KMS Signer provider. Valid options are secret, memory or any supported KMS provider defined by go-cloud style URI
	//+kubebuilder:default:=secret
	KMS string `json:"kms,omitempty"`

	// Password to decrypt signer private key
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	// Reference to signer private key
	//+optional
	KeyRef *SecretKeySelector `json:"keyRef,omitempty"`
}

type RekorSearchUI struct {
	//Enable RekorSearchUI deployment
	Enabled bool `json:"enabled,omitempty"`
}

// RekorStatus defines the observed state of Rekor
type RekorStatus struct {
	Url                string `json:"url,omitempty"`
	Phase              Phase  `json:"phase,omitempty"`
	TreeID             *int64 `json:"treeID,omitempty"`
	RekorSearchUIPhase Phase  `json:"rekorSearchUIPhase,omitempty"`
	RekorSearchUIUrl   string `json:"rekorSearchUIUrl,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The component phase"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

// Rekor is the Schema for the rekors API
type Rekor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RekorSpec   `json:"spec,omitempty"`
	Status RekorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RekorList contains a list of Rekor
type RekorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rekor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rekor{}, &RekorList{})
}
