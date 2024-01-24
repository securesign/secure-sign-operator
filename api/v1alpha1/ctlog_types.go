package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CTlogSpec defines the desired state of CTlog component
type CTlogSpec struct {
	// The ID of a Trillian tree that stores the log data.
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`

	// The private key used for signing STHs etc.
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`

	// Password to decrypt private key
	//+optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// The public key matching the private key (if both are present). It is
	// used only by mirror logs for verifying the source log's signatures, but can
	// be specified for regular logs as well for the convenience of test tools.
	//+optional
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`

	// List of secrets containing root certificates that are acceptable to the log.
	// The certs are served through get-roots endpoint. Optional in mirrors.
	//+optional
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	Phase Phase `json:"phase"`

	TreeID *int64 `json:"treeID,omitempty"`
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
