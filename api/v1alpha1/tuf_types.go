package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TufSpec defines the desired state of Tuf
type TufSpec struct {
	External bool   `json:"external,omitempty"`
	Image    string `json:"image,omitempty"`
}

// TufStatus defines the observed state of Tuf
type TufStatus struct {
	Url   string `json:"url,omitempty"`
	Phase Phase  `json:"Phase"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Tuf is the Schema for the tufs API
type Tuf struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TufSpec   `json:"spec,omitempty"`
	Status TufStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TufList contains a list of Tuf
type TufList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tuf `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tuf{}, &TufList{})
}
