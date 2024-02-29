package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TufSpec defines the desired state of Tuf
type TufSpec struct {
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//+kubebuilder:default:=80
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	Port int32 `json:"port,omitempty"`
	// List of TUF targets which will be added to TUF root
	//+kubebuilder:default:={{name: rekor.pub},{name: ctfe.pub},{name: fulcio_v1.crt.pem}}
	//+kubebuilder:validation:MinItems:=1
	Keys []TufKey `json:"keys,omitempty"`
}

type TufKey struct {
	// File name which will be used as TUF target.
	//+required
	//+kubebuilder:validation:Pattern:="^[-._a-zA-Z0-9]+$"
	Name string `json:"name"`
	// Reference to secret object
	// If it is unset, the operator will try to autoconfigure secret reference, by searching secrets in namespace which
	// contain `rhtas.redhat.com/$name` label.
	//+optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

// TufStatus defines the observed state of Tuf
type TufStatus struct {
	Keys []TufKey `json:"keys,omitempty"`
	Url  string   `json:"url,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The component status"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

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
