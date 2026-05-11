// This file contains embedded code derived from the Sigstore Fulcio project.
//
// Original Project:   Sigstore Fulcio
// Original Repository: https://github.com/sigstore/fulcio
// Original License:   Apache License, Version 2.0

package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FulcioSpec defines the desired state of Fulcio
type FulcioSpec struct {
	PodRequirements `json:",inline"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	// Ctlog service configuration
	//+optional
	//+kubebuilder:default:={prefix: trusted-artifact-signer}
	Ctlog CtlogService `json:"ctlog,omitempty"`
	// Fulcio Configuration
	//+required
	Config FulcioConfig `json:"config"`
	// Certificate configuration
	Certificate FulcioCert `json:"certificate"`
	//Enable Service monitors for fulcio
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

// CAType specifies how the Fulcio CA key is managed.
// +kubebuilder:validation:Enum=file;pkcs11
type CAType string

const (
	CATypeFile   CAType = "file"
	CATypePKCS11 CAType = "pkcs11"
)

// FulcioCert defines fields for system-generated certificate
// +kubebuilder:validation:XValidation:rule=(has(self.caRef) || self.organizationName != "" || (has(self.caType) && self.caType == 'pkcs11')),message=organizationName cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caRef) || has(self.privateKeyRef) || (has(self.caType) && self.caType == 'pkcs11')),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != 'pkcs11' || has(self.pkcs11)),message=pkcs11 config is required when caType is pkcs11
type FulcioCert struct {
	// CAType selects the CA backend: "file" (default) or "pkcs11".
	//+optional
	//+kubebuilder:default:="file"
	CAType CAType `json:"caType,omitempty"`

	// PKCS11 configuration. Required when caType is "pkcs11".
	//+optional
	PKCS11 *PKCS11Config `json:"pkcs11,omitempty"`

	// Reference to CA private key (file CA only)
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Reference to password to encrypt CA private key (file CA only)
	//+optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// Reference to CA certificate (file CA only)
	//+optional
	CARef *SecretKeySelector `json:"caRef,omitempty"`

	//+optional
	// CommonName specifies the common name for the Fulcio certificate.
	// If not provided, the common name will default to the host name.
	CommonName string `json:"commonName,omitempty"`
	//+optional
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	OrganizationEmail string `json:"organizationEmail,omitempty"`
}

// PKCS11Config configures Fulcio to use a PKCS#11 backend.
// The operator is vendor-agnostic: the init container is a plugin that provisions
// the token and makes the .so library available. SoftHSM, Thales Luna, AWS CloudHSM,
// or any PKCS#11-compliant backend can be used by supplying the appropriate init image
// and vendor-specific configuration.
//
// Two modes are supported:
//   - Inline: provide pin, tokenLabel, libraryPath — operator generates Secrets
//   - Reference: provide credentialsRef, pkcs11ConfigRef — user pre-creates Secrets
//
// +kubebuilder:validation:XValidation:rule="(has(self.pin) && has(self.tokenLabel) && has(self.libraryPath)) || (has(self.credentialsRef) && has(self.pkcs11ConfigRef))",message="provide either inline config (pin + tokenLabel + libraryPath) or references (credentialsRef + pkcs11ConfigRef)"
type PKCS11Config struct {
	// Init container plugin that provisions the PKCS#11 token and library.
	//+required
	InitContainer PKCS11InitContainer `json:"initContainer"`

	// HSM PIN value. The operator creates a Secret from this.
	// Mutually exclusive with credentialsRef.
	//+optional
	Pin string `json:"pin,omitempty"`

	// PKCS#11 token label (e.g. "fulcio"). Used to build crypto11.conf.
	// Mutually exclusive with pkcs11ConfigRef.
	//+optional
	TokenLabel string `json:"tokenLabel,omitempty"`

	// Full path to the PKCS#11 .so library inside the init container image.
	// The operator copies this library to the shared volume automatically.
	// Mutually exclusive with pkcs11ConfigRef.
	//+optional
	LibraryPath string `json:"libraryPath,omitempty"`

	// Reference to a pre-existing Secret containing crypto11.conf.
	// Takes precedence over inline fields (tokenLabel, libraryPath, pin).
	//+optional
	PKCS11ConfigRef *SecretKeySelector `json:"pkcs11ConfigRef,omitempty"`

	// Reference to a pre-existing Secret containing the HSM PIN.
	// Takes precedence over inline pin field.
	//+optional
	CredentialsRef *SecretKeySelector `json:"credentialsRef,omitempty"`

	// Additional environment variables for the main Fulcio server container.
	// Use this for vendor-specific env vars (e.g. SOFTHSM2_CONF for SoftHSM).
	//+optional
	ServerEnv []PKCS11EnvVar `json:"serverEnv,omitempty"`

	// PKCS#11 key parameters.
	//+optional
	//+kubebuilder:default:={id: 99, label: "PKCS11CA", algorithm: "EC:secp384r1"}
	KeyConfig PKCS11KeyConfig `json:"keyConfig,omitempty"`

	// Root CA subject for the createca init container.
	//+optional
	//+kubebuilder:default:={org: "RHTAS"}
	RootCA PKCS11RootCA `json:"rootCA,omitempty"`

	// Persistent storage for HSM tokens (key survives pod restarts).
	// When nil, an emptyDir is used (key is regenerated on every pod restart).
	// Not needed for hardware HSMs where keys live on the device.
	//+optional
	Persistence *Pvc `json:"persistence,omitempty"`
}

// PKCS11InitContainer defines the vendor-specific init container plugin.
type PKCS11InitContainer struct {
	// Container image that provisions the HSM token and library.
	//+required
	Image string `json:"image"`

	// Additional environment variables for the init container (vendor-specific).
	// The operator always injects HSM_PIN automatically.
	//+optional
	Env []PKCS11EnvVar `json:"env,omitempty"`

	// Additional volumes to mount into the init container (vendor-specific configs).
	//+optional
	Volumes []PKCS11Volume `json:"volumes,omitempty"`
}

// PKCS11EnvVar defines a simple name/value environment variable.
type PKCS11EnvVar struct {
	//+required
	Name string `json:"name"`
	//+required
	Value string `json:"value"`
}

// PKCS11Volume defines a volume to mount into the init container and/or server container.
// +kubebuilder:validation:XValidation:rule="[has(self.configMapName), has(self.secretName), has(self.inlineData)].filter(x, x).size() <= 1",message="only one of configMapName, secretName, or inlineData may be set"
type PKCS11Volume struct {
	// Volume name (must be unique within the pod).
	//+required
	Name string `json:"name"`
	// Mount path inside the container.
	//+required
	MountPath string `json:"mountPath"`
	// ReadOnly mount. Defaults to true for config volumes.
	//+optional
	//+kubebuilder:default:=true
	ReadOnly bool `json:"readOnly,omitempty"`
	// ConfigMap name to mount (mutually exclusive with secretName and inlineData).
	//+optional
	ConfigMapName string `json:"configMapName,omitempty"`
	// Secret name to mount (mutually exclusive with configMapName and inlineData).
	//+optional
	SecretName string `json:"secretName,omitempty"`
	// Inline data — operator creates a ConfigMap from this map.
	// Mutually exclusive with configMapName and secretName.
	//+optional
	InlineData map[string]string `json:"inlineData,omitempty"`
}

// PKCS11KeyConfig defines the HSM key parameters.
type PKCS11KeyConfig struct {
	// PKCS#11 object ID for the CA root key.
	//+kubebuilder:default:=99
	ID int `json:"id,omitempty"`
	// Key label in the HSM token.
	//+kubebuilder:default:="PKCS11CA"
	Label string `json:"label,omitempty"`
	// Key algorithm (passed to pkcs11-tool --key-type).
	//+kubebuilder:default:="EC:secp384r1"
	Algorithm string `json:"algorithm,omitempty"`
}

// PKCS11RootCA defines the root CA subject for createca.
type PKCS11RootCA struct {
	//+kubebuilder:default:="RHTAS"
	Org string `json:"org,omitempty"`
	//+optional
	Country string `json:"country,omitempty"`
	//+optional
	Locality string `json:"locality,omitempty"`
	//+optional
	Province string `json:"province,omitempty"`
}

// FulcioConfig configuration of OIDC issuers
// +kubebuilder:validation:XValidation:rule=(has(self.OIDCIssuers) && (size(self.OIDCIssuers) > 0)) || (has(self.MetaIssuers) && (size(self.MetaIssuers) > 0)),message=At least one of OIDCIssuers or MetaIssuers must be defined
type FulcioConfig struct {
	// OIDC Configuration
	// +optional
	OIDCIssuers []OIDCIssuer `json:"OIDCIssuers,omitempty" yaml:"oidc-issuers,omitempty"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// +optional
	MetaIssuers []OIDCIssuer `json:"MetaIssuers,omitempty" yaml:"meta-issuers,omitempty"`

	// Metadata used for the CIProvider identity provider principal
	CIIssuerMetadata []CIIssuerMetadata `json:"CIIssuerMetadata,omitempty" yaml:"ci-issuer-metadata,omitempty"`
}

type OIDCIssuer struct {
	IssuerURL         string `json:"IssuerURL,omitempty" yaml:"issuer-url,omitempty"`
	Issuer            string `json:"Issuer" yaml:"issuer"`
	ClientID          string `json:"ClientID" yaml:"client-id"`
	Type              string `json:"Type" yaml:"type"`
	CIProvider        string `json:"CIProvider,omitempty" yaml:"ci-provider,omitempty"`
	IssuerClaim       string `json:"IssuerClaim,omitempty" yaml:"issuer-claim,omitempty"`
	SubjectDomain     string `json:"SubjectDomain,omitempty" yaml:"subject-domain,omitempty"`
	SPIFFETrustDomain string `json:"SPIFFETrustDomain,omitempty" yaml:"spiffe-trust-domain,omitempty"`
	ChallengeClaim    string `json:"ChallengeClaim,omitempty" yaml:"challenge-claim,omitempty"`
}

type CIIssuerMetadata struct {
	IssuerName                     string            `json:"IssuerName" yaml:"issuer-name"`
	DefaultTemplateValues          map[string]string `json:"DefaultTemplateValues,omitempty" yaml:"default-template-values,omitempty"`
	ExtensionTemplates             Extensions        `json:"ExtensionTemplates,omitempty" yaml:"extension-templates,omitempty"`
	SubjectAlternativeNameTemplate string            `json:"SubjectAlternativeNameTemplate,omitempty" yaml:"subject-alternative-name-template,omitempty"`
}

// Extensions contains all custom x509 extensions defined by Fulcio
type Extensions struct {
	BuildSignerURI                      string `json:"BuildSignerURI,omitempty" yaml:"build-signer-uri,omitempty"`
	BuildSignerDigest                   string `json:"BuildSignerDigest,omitempty" yaml:"build-signer-digest,omitempty"`
	RunnerEnvironment                   string `json:"RunnerEnvironment,omitempty" yaml:"runner-environment,omitempty"`
	SourceRepositoryURI                 string `json:"SourceRepositoryURI,omitempty" yaml:"source-repository-uri,omitempty"`
	SourceRepositoryDigest              string `json:"SourceRepositoryDigest,omitempty" yaml:"source-repository-digest,omitempty"`
	SourceRepositoryRef                 string `json:"SourceRepositoryRef,omitempty" yaml:"source-repository-ref,omitempty"`
	SourceRepositoryIdentifier          string `json:"SourceRepositoryIdentifier,omitempty" yaml:"source-repository-identifier,omitempty"`
	SourceRepositoryOwnerURI            string `json:"SourceRepositoryOwnerURI,omitempty" yaml:"source-repository-owner-uri,omitempty"`
	SourceRepositoryOwnerIdentifier     string `json:"SourceRepositoryOwnerIdentifier,omitempty" yaml:"source-repository-owner-identifier,omitempty"`
	BuildConfigURI                      string `json:"BuildConfigURI,omitempty" yaml:"build-config-uri,omitempty"`
	BuildConfigDigest                   string `json:"BuildConfigDigest,omitempty" yaml:"build-config-digest,omitempty"`
	BuildTrigger                        string `json:"BuildTrigger,omitempty" yaml:"build-trigger,omitempty"`
	RunInvocationURI                    string `json:"RunInvocationURI,omitempty" yaml:"run-invocation-uri,omitempty"`
	SourceRepositoryVisibilityAtSigning string `json:"SourceRepositoryVisibilityAtSigning,omitempty" yaml:"source-repository-visibility-at-signing,omitempty"`
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
//+kubebuilder:storageversion
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

func (i *Fulcio) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Fulcio) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Fulcio) GetTrustedCA() *LocalObjectReference {
	if i.Spec.TrustedCA != nil {
		return i.Spec.TrustedCA
	}

	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{
			Name: v,
		}
	}

	return nil
}
