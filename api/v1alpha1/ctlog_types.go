package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CTlogSpec defines the desired state of CTlog component
type CTlogSpec struct {
	// Certificate configuration
	Certificate CtlogCert `json:"certificate,omitempty"`
}

type CtlogCert struct {
	Create bool `json:"create"`
	//Name of the secret the ctlog private key is stored in
	SecretName string `json:"secretName,omitempty"` // +kubebuilder:validation:+optional
	//Key for the private key inside the secret
	SecretKey string `json:"secretKey,omitempty"` // +kubebuilder:validation:+optional
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	Phase Phase `json:"phase"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The component phase"

// CTlog is the Schema for the ctlogs API
type CTlog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CTlogSpec   `json:"spec,omitempty"`
	Status CTlogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CTlogList contains a list of CTlog
type CTlogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CTlog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CTlog{}, &CTlogList{})
}
