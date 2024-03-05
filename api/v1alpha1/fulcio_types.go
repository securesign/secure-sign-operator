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
	//+required
	Config FulcioConfig `json:"config"`
	// Certificate configuration
	Certificate FulcioCert `json:"certificate"`
	//Enable Service monitors for fulcio
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
}

// FulcioCert defines fields for system-generated certificate
// +kubebuilder:validation:XValidation:rule=(has(self.caRef) || self.commonName != ""),message=commonName cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type FulcioCert struct {
	// Reference to CA private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to password to encrypt CA private key
	//+optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// Reference to CA certificate
	//+optional
	CARef *SecretKeySelector `json:"caRef,omitempty"`

	//+optional
	CommonName string `json:"commonName,omitempty"`
	//+optional
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	OrganizationEmail string `json:"organizationEmail,omitempty"`
}

type FulcioConfig struct {
	//+kubebuilder:validation:MinProperties:=1
	OIDCIssuers map[string]OIDCIssuer `json:"OIDCIssuers"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// Where * can match a single hostname or URI path parts
	// (in particular, no '.' or '/' are permitted, among
	// other special characters)  Some examples we want to match:
	// * https://oidc.eks.us-west-2.amazonaws.com/id/B02C93B6A2D30341AD01E1B6D48164CB
	// * https://container.googleapis.com/v1/projects/mattmoor-credit/locations/us-west1-b/clusters/tenant-cluster
	// +optional
	MetaIssuers map[string]OIDCIssuer `json:"MetaIssuers,omitempty"`
}

type OIDCIssuer struct {
	// The expected issuer of an OIDC token
	IssuerURL string `json:"IssuerURL,omitempty"`
	// The expected client ID of the OIDC token
	//+required
	ClientID string `json:"ClientID"`
	// Used to determine the subject of the certificate and if additional
	// certificate values are needed
	//+required
	Type string `json:"Type"`
	// Optional, if the issuer is in a different claim in the OIDC token
	IssuerClaim string `json:"IssuerClaim,omitempty"`
	// The domain that must be present in the subject for 'uri' issuer types
	// Also used to create an email for 'username' issuer types
	SubjectDomain string `json:"SubjectDomain,omitempty"`
	// SPIFFETrustDomain specifies the trust domain that 'spiffe' issuer types
	// issue ID tokens for. Tokens with a different trust domain will be
	// rejected.
	SPIFFETrustDomain string `json:"SPIFFETrustDomain,omitempty"`
	// Optional, the challenge claim expected for the issuer
	// Set if using a custom issuer
	ChallengeClaim string `json:"ChallengeClaim,omitempty"`
}

// FulcioStatus defines the observed state of Fulcio
type FulcioStatus struct {
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Certificate     *FulcioCert           `json:"certificate,omitempty"`
	Url             string                `json:"url,omitempty"`
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
