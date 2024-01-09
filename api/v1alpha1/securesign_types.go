package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecuresignSpec defines the desired state of Securesign
type SecuresignSpec struct {
	Rekor    RekorSpec    `json:"rekor,omitempty"`
	Fulcio   FulcioSpec   `json:"fulcio,omitempty"`
	Trillian TrillianSpec `json:"trillian,omitempty"`
	Tuf      TufSpec      `json:"tuf,omitempty"`
	Ctlog    CTlogSpec    `json:"ctlog,omitempty"`
}

// SecuresignStatus defines the observed state of Securesign
type SecuresignStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Trillian string `json:"trillian"`
	Fulcio   string `json:"fulcio"`
	Tuf      string `json:"tuf"`
	CTlog    string `json:"ctlog"`
	Rekor    string `json:"rekor"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Securesign is the Schema for the securesigns API
type Securesign struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecuresignSpec   `json:"spec,omitempty"`
	Status SecuresignStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecuresignList contains a list of Securesign
type SecuresignList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Securesign `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Securesign{}, &SecuresignList{})
}
