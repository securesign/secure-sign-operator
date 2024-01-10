package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RekorSpec defines the desired state of Rekor
type RekorSpec struct {
	// Define whether you want to export service or not
	External bool `json:"external,omitempty"`
	// Persistent volume claim name to bound with Rekor component
	PvcName string `json:"pvcName,omitempty"`
	// Certificate configuration
	Certificate RekorCert `json:"certificate,omitempty"`
	//Enable Service monitors for rekor
	Monitoring bool `json:"monitoring,omitempty"`
}

type RekorCert struct {
	// Generate certificate
	Create bool `json:"create"`
	// Enter secret name for your keys and certificate (will be generated in case of `create=true`)
	// Required fields: private
	SecretName string `json:"secretName"`
}

// RekorStatus defines the observed state of Rekor
type RekorStatus struct {
	Url   string `json:"url,omitempty"`
	Phase Phase  `json:"phase,omitempty"`
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
