package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FulcioSpec defines the desired state of Fulcio
type FulcioSpec struct {
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	// OIDC issuer configuration
	OidcIssuers map[string]OidcIssuer `json:"oidcIssuers"`
	// Certificate configuration
	Certificate FulcioCert `json:"certificate,omitempty"`
	//Enable Service monitors for fulcio
	Monitoring bool `json:"monitoring,omitempty"`
}

// FulcioCert defines fields for system-generated certificate
type FulcioCert struct {
	Create bool `json:"create"`
	// Enter secret name for your keys and certificate (will be generated in case of the `create=true`)
	// Required fields are: public,private,cert,password
	SecretName        string `json:"secretName"`
	OrganizationName  string `json:"organizationName,omitempty"`  // +kubebuilder:validation:+optional
	OrganizationEmail string `json:"organizationEmail,omitempty"` // +kubebuilder:validation:+optional
}

type OidcIssuer struct {
	ClientID  string `json:"ClientID"`
	IssuerURL string `json:"IssuerURL"`
	Type      string `json:"Type"`
}

// FulcioStatus defines the observed state of Fulcio
type FulcioStatus struct {
	Url   string `json:"url,omitempty"`
	Phase Phase  `json:"phase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The component phase"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

// Fulcio is the Schema for the fulcios API
type Fulcio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FulcioSpec   `json:"spec,omitempty"`
	Status FulcioStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FulcioList contains a list of Fulcio
type FulcioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fulcio `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fulcio{}, &FulcioList{})
}
