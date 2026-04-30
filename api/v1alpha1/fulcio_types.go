// This file contains embedded code derived from the Sigstore Fulcio project.
//
// Original Project:   Sigstore Fulcio
// Original Repository: https://github.com/sigstore/fulcio
// Original License:   Apache License, Version 2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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
// +kubebuilder:validation:XValidation:rule=(!has(self.caRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
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
	// The operator mounts two shared volumes into this container:
	//   - /var/lib/hsm/tokens  (persistent or emptyDir for token data)
	// The operator also injects HSM_PIN automatically.
	// The operator handles copying the PKCS#11 library to the shared lib volume
	// via a separate init container using libraryPath.
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

	// Full path to the PKCS#11 .so library inside the init container image
	// (e.g. "/usr/lib64/pkcs11/libsofthsm2.so", "/usr/lib/libCryptoki2_64.so").
	// The operator copies this library to the shared volume automatically
	// and derives the filename for crypto11.conf.
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
// This container is responsible for:
//  1. Initializing the HSM token (e.g. softhsm2-util --init-token)
//  2. Generating the key pair (e.g. pkcs11-tool --keypairgen)
//
// The operator handles copying the PKCS#11 library automatically via libraryPath.
type PKCS11InitContainer struct {
	// Container image that provisions the HSM token and library.
	// Examples: quay.io/rh-ee-sacm/softhsm-init:latest (SoftHSM),
	// or a vendor-specific image for hardware HSMs.
	//+required
	Image string `json:"image"`

	// Additional environment variables for the init container (vendor-specific).
	// The operator always injects HSM_PIN and EXPORT_LIB_DIR automatically.
	//+optional
	Env []PKCS11EnvVar `json:"env,omitempty"`

	// Additional volumes to mount into the init container (vendor-specific configs).
	// Use this for things like softhsm2.conf ConfigMaps, vendor config files, etc.
	// These volumes are also added to the pod spec.
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
	// Keys are filenames, values are file contents.
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
// NOTE: the below validation (and a similar one for MetaIssuers) would be great to have, but unfortunately it can't be used because compiling it yields:
// "Forbidden: estimated rule cost exceeds budget by factor of more than 100x". It is turned off for now, maybe this can be fixed in the future.
// Note that the error message also suggests to use MaxItems/MaxLength on the involved arrays/strings, but that doesn't seem to work either.
// kubebuilder:validation:XValidation:rule="!has(self.OIDCIssuers) || has(self.OIDCIssuers) && self.OIDCIssuers.all(i, (!has(i.CIProvider) || (has(i.CIProvider) && i.CIProvider in self.CIIssuerMetadata.map(n, n.IssuerName))))",message=All CIProvider values of OIDCIssuers must be present in CIIssuerMetadata
type FulcioConfig struct {
	// OIDC Configuration
	// +optional
	OIDCIssuers []OIDCIssuer `json:"OIDCIssuers,omitempty" yaml:"oidc-issuers,omitempty"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// Where * can match a single hostname or URI path parts
	// (in particular, no '.' or '/' are permitted, among
	// other special characters)  Some examples we want to match:
	// * https://oidc.eks.us-west-2.amazonaws.com/id/B02C93B6A2D30341AD01E1B6D48164CB
	// * https://container.googleapis.com/v1/projects/mattmoor-credit/locations/us-west1-b/clusters/tenant-cluster
	// +optional
	MetaIssuers []OIDCIssuer `json:"MetaIssuers,omitempty" yaml:"meta-issuers,omitempty"`

	// Metadata used for the CIProvider identity provider principal
	CIIssuerMetadata []CIIssuerMetadata `json:"CIIssuerMetadata,omitempty" yaml:"ci-issuer-metadata,omitempty"`
}

type OIDCIssuer struct {
	// The expected issuer of an OIDC token
	IssuerURL string `json:"IssuerURL,omitempty" yaml:"issuer-url,omitempty"`
	// The expected issuer of an OIDC token
	//+required
	Issuer string `json:"Issuer" yaml:"issuer"`
	//+required
	ClientID string `json:"ClientID" yaml:"client-id"`
	// Used to determine the subject of the certificate and if additional
	// certificate values are needed
	//+required
	Type string `json:"Type" yaml:"type"`
	// CIProvider is an optional configuration to map token claims to extensions for CI workflows
	CIProvider string `json:"CIProvider,omitempty" yaml:"ci-provider,omitempty"`
	// Optional, if the issuer is in a different claim in the OIDC token
	IssuerClaim string `json:"IssuerClaim,omitempty" yaml:"issuer-claim,omitempty"`
	// The domain that must be present in the subject for 'uri' issuer types
	// Also used to create an email for 'username' issuer types
	SubjectDomain string `json:"SubjectDomain,omitempty" yaml:"subject-domain,omitempty"`
	// SPIFFETrustDomain specifies the trust domain that 'spiffe' issuer types
	// issue ID tokens for. Tokens with a different trust domain will be
	// rejected.
	SPIFFETrustDomain string `json:"SPIFFETrustDomain,omitempty" yaml:"spiffe-trust-domain,omitempty"`
	// Optional, the challenge claim expected for the issuer
	// Set if using a custom issuer
	ChallengeClaim string `json:"ChallengeClaim,omitempty" yaml:"challenge-claim,omitempty"`
}

type CIIssuerMetadata struct {
	// Name of the issuer
	//+required
	IssuerName string `json:"IssuerName" yaml:"issuer-name"`
	// Defaults contains key-value pairs that can be used for filling the templates from ExtensionTemplates
	// If a key cannot be found on the token claims, the template will use the defaults
	DefaultTemplateValues map[string]string `json:"DefaultTemplateValues,omitempty" yaml:"default-template-values,omitempty"`
	// ExtensionTemplates contains a mapping between certificate extension and token claim
	// Provide either strings following https://pkg.go.dev/text/template syntax,
	// e.g "{{ .url }}/{{ .repository }}"
	// or non-templated strings with token claim keys to be replaced,
	// e.g "job_workflow_sha"
	ExtensionTemplates Extensions `json:"ExtensionTemplates,omitempty" yaml:"extension-templates,omitempty"`
	// Template for the Subject Alternative Name extension
	// It's typically the same value as Build Signer URI
	SubjectAlternativeNameTemplate string `json:"SubjectAlternativeNameTemplate,omitempty" yaml:"subject-alternative-name-template,omitempty"`
}

// Extensions contains all custom x509 extensions defined by Fulcio
type Extensions struct {
	// Reference to specific build instructions that are responsible for signing.
	BuildSignerURI string `json:"BuildSignerURI,omitempty" yaml:"build-signer-uri,omitempty"` // 1.3.6.1.4.1.57264.1.9

	// Immutable reference to the specific version of the build instructions that is responsible for signing.
	BuildSignerDigest string `json:"BuildSignerDigest,omitempty" yaml:"build-signer-digest,omitempty"` // 1.3.6.1.4.1.57264.1.10

	// Specifies whether the build took place in platform-hosted cloud infrastructure or customer/self-hosted infrastructure.
	RunnerEnvironment string `json:"RunnerEnvironment,omitempty" yaml:"runner-environment,omitempty"` // 1.3.6.1.4.1.57264.1.11

	// Source repository URL that the build was based on.
	SourceRepositoryURI string `json:"SourceRepositoryURI,omitempty" yaml:"source-repository-uri,omitempty"` // 1.3.6.1.4.1.57264.1.12

	// Immutable reference to a specific version of the source code that the build was based upon.
	SourceRepositoryDigest string `json:"SourceRepositoryDigest,omitempty" yaml:"source-repository-digest,omitempty"` // 1.3.6.1.4.1.57264.1.13

	// Source Repository Ref that the build run was based upon.
	SourceRepositoryRef string `json:"SourceRepositoryRef,omitempty" yaml:"source-repository-ref,omitempty"` // 1.3.6.1.4.1.57264.1.14

	// Immutable identifier for the source repository the workflow was based upon.
	SourceRepositoryIdentifier string `json:"SourceRepositoryIdentifier,omitempty" yaml:"source-repository-identifier,omitempty"` // 1.3.6.1.4.1.57264.1.15

	// Source repository owner URL of the owner of the source repository that the build was based on.
	SourceRepositoryOwnerURI string `json:"SourceRepositoryOwnerURI,omitempty" yaml:"source-repository-owner-uri,omitempty"` // 1.3.6.1.4.1.57264.1.16

	// Immutable identifier for the owner of the source repository that the workflow was based upon.
	SourceRepositoryOwnerIdentifier string `json:"SourceRepositoryOwnerIdentifier,omitempty" yaml:"source-repository-owner-identifier,omitempty"` // 1.3.6.1.4.1.57264.1.17

	// Build Config URL to the top-level/initiating build instructions.
	BuildConfigURI string `json:"BuildConfigURI,omitempty" yaml:"build-config-uri,omitempty"` // 1.3.6.1.4.1.57264.1.18

	// Immutable reference to the specific version of the top-level/initiating build instructions.
	BuildConfigDigest string `json:"BuildConfigDigest,omitempty" yaml:"build-config-digest,omitempty"` // 1.3.6.1.4.1.57264.1.19

	// Event or action that initiated the build.
	BuildTrigger string `json:"BuildTrigger,omitempty" yaml:"build-trigger,omitempty"` // 1.3.6.1.4.1.57264.1.20

	// Run Invocation URL to uniquely identify the build execution.
	RunInvocationURI string `json:"RunInvocationURI,omitempty" yaml:"run-invocation-uri,omitempty"` // 1.3.6.1.4.1.57264.1.21

	// Source repository visibility at the time of signing the certificate.
	SourceRepositoryVisibilityAtSigning string `json:"SourceRepositoryVisibilityAtSigning,omitempty" yaml:"source-repository-visibility-at-signing,omitempty"` // 1.3.6.1.4.1.57264.1.22
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
