// This file contains embedded code derived from the Sigstore Fulcio project.
//
// Original Project:   Sigstore Fulcio
// Original Repository: https://github.com/sigstore/fulcio
// Original License:   Apache License, Version 2.0

package v1

import (
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CAType specifies the certificate authority backend for Fulcio.
// +kubebuilder:validation:Enum:=file;pkcs11
type CAType string

const (
	// CATypeFile uses file-based certificate authority (default).
	CATypeFile CAType = "file"
	// CATypePKCS11 uses a PKCS#11 hardware security module.
	CATypePKCS11 CAType = "pkcs11"
)

// PKCS11InitContainerSpec defines a curated subset of corev1.Container for PKCS#11 init containers.
// These containers run before the main Fulcio server to perform vendor-specific HSM initialization.
type PKCS11InitContainerSpec struct {
	// Name of the init container. Must be unique within the pod.
	//+required
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	Name string `json:"name"`
	// Container image name.
	//+required
	Image string `json:"image"`
	// Entrypoint array. Not executed within a shell.
	//+optional
	Command []string `json:"command,omitempty"`
	// Arguments to the entrypoint.
	//+optional
	Args []string `json:"args,omitempty"`
	// List of environment variables to set in the container.
	//+optional
	Env []core.EnvVar `json:"env,omitempty"`
	// List of sources to populate environment variables in the container.
	//+optional
	EnvFrom []core.EnvFromSource `json:"envFrom,omitempty"`
	// Pod volumes to mount into the container's filesystem.
	//+optional
	VolumeMounts []core.VolumeMount `json:"volumeMounts,omitempty"`
	// Compute Resources required by this container.
	//+optional
	Resources *core.ResourceRequirements `json:"resources,omitempty"`
	// SecurityContext defines the security options the container should be run with.
	//+optional
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty"`
	// Image pull policy.
	//+optional
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// PKCS11KeyConfig defines the key configuration for PKCS#11 CA operations.
type PKCS11KeyConfig struct {
	// The PKCS#11 key ID used to identify the HSM key slot.
	// This value is passed to Fulcio via --hsm-caroot-id.
	//+required
	//+kubebuilder:validation:Minimum=0
	ID int `json:"id"`
	// Optional human-readable label for the key.
	//+optional
	Label string `json:"label,omitempty"`
}

// FulcioPKCS11Config defines the PKCS#11 HSM configuration for Fulcio.
type FulcioPKCS11Config struct {
	// Init containers that run before the Fulcio server for vendor-specific HSM initialization.
	//+optional
	InitContainers []PKCS11InitContainerSpec `json:"initContainers,omitempty"`
	// Additional pod-level volumes needed for HSM connectivity.
	//+optional
	Volumes []core.Volume `json:"volumes,omitempty"`
	// Reference to a Secret containing the HSM PIN credential.
	//+required
	CredentialsRef SecretKeySelector `json:"credentialsRef"`
	// Reference to a Secret containing the crypto11.conf PKCS#11 configuration file.
	//+required
	PKCS11ConfigRef SecretKeySelector `json:"pkcs11ConfigRef"`
	// Additional environment variables for the Fulcio server container.
	//+optional
	ServerEnv []core.EnvVar `json:"serverEnv,omitempty"`
	// Additional volume mounts for the Fulcio server container.
	//+optional
	ServerVolumeMounts []core.VolumeMount `json:"serverVolumeMounts,omitempty"`
	// Key configuration for the PKCS#11 CA.
	//+required
	KeyConfig PKCS11KeyConfig `json:"keyConfig"`
}

// FulcioPKCS11Status records the observed PKCS#11 configuration references.
type FulcioPKCS11Status struct {
	// Last observed credentialsRef.
	//+optional
	CredentialsRef *SecretKeySelector `json:"credentialsRef,omitempty"`
	// Last observed pkcs11ConfigRef.
	//+optional
	PKCS11ConfigRef *SecretKeySelector `json:"pkcs11ConfigRef,omitempty"`
}

// FulcioSpec defines the desired state of Fulcio
type FulcioSpec struct {
	// Pod resource requirements and scheduling constraints
	PodRequirements `json:",inline"`
	// Service account configuration for the Fulcio deployment
	ServiceAccountConfig `json:",inline"`
	// Define whether you want to export service or not
	//+optional
	Ingress Ingress `json:"ingress,omitempty"`
	// Ctlog service configuration
	//+optional
	Ctlog CtlogService `json:"ctlog,omitempty"`
	// Fulcio Configuration
	//+required
	Config FulcioConfig `json:"config"`
	// Certificate configuration
	//+required
	Certificate FulcioCert `json:"certificate"`
	//Enable Service monitors for fulcio
	//+optional
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

// FulcioCert defines certificate configuration for Fulcio CA
// +kubebuilder:validation:XValidation:rule=(has(self.caRef) || has(self.organizationName) && self.organizationName != "" || (has(self.caType) && self.caType == "pkcs11")),message=organizationName cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caRef) || has(self.privateKeyRef) || (has(self.caType) && self.caType == "pkcs11")),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != "pkcs11" || has(self.pkcs11)),message=pkcs11 config is required when caType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != "pkcs11" || has(self.caRef)),message=caRef is required when caType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != "pkcs11" || !has(self.privateKeyRef)),message=privateKeyRef must not be set when caType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != "pkcs11" || !has(self.privateKeyPasswordRef)),message=privateKeyPasswordRef must not be set when caType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.caType) || self.caType != "file" || !has(self.pkcs11)),message=pkcs11 config must not be set when caType is file
type FulcioCert struct {
	// CA backend type. Defaults to "file" if not specified.
	//+optional
	//+kubebuilder:default:=file
	CAType CAType `json:"caType,omitempty"`

	// Reference to CA private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Deprecated: Legacy PEM encryption as specified in RFC 1423 is insecure by design
	// and not FIPS-compliant. Auto-generated keys are no longer password-encrypted;
	// this field is retained only for backward compatibility with existing user-provided
	// encrypted keys. Kubernetes Secrets provide encryption-at-rest.
	// +optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// Reference to CA certificate
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

	// PKCS#11 HSM configuration. Required when caType is "pkcs11".
	//+optional
	PKCS11 *FulcioPKCS11Config `json:"pkcs11,omitempty"`
}

// FulcioConfig configuration of OIDC issuers
// +kubebuilder:validation:XValidation:rule=(has(self.oidcIssuers) && (size(self.oidcIssuers) > 0)) || (has(self.metaIssuers) && (size(self.metaIssuers) > 0)),message=At least one of oidcIssuers or metaIssuers must be defined
// NOTE: the below validation (and a similar one for MetaIssuers) would be great to have, but unfortunately it can't be used because compiling it yields:
// "Forbidden: estimated rule cost exceeds budget by factor of more than 100x". It is turned off for now, maybe this can be fixed in the future.
// Note that the error message also suggests to use MaxItems/MaxLength on the involved arrays/strings, but that doesn't seem to work either.
// kubebuilder:validation:XValidation:rule="!has(self.oidcIssuers) || has(self.oidcIssuers) && self.oidcIssuers.all(i, (!has(i.ciProvider) || (has(i.ciProvider) && i.ciProvider in self.ciIssuerMetadata.map(n, n.issuerName))))",message=All CIProvider values of OIDCIssuers must be present in CIIssuerMetadata
type FulcioConfig struct {
	// OIDC Configuration
	// +optional
	// +listType=map
	// +listMapKey=issuer
	OIDCIssuers []OIDCIssuer `json:"oidcIssuers,omitempty"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// Where * can match a single hostname or URI path parts
	// (in particular, no '.' or '/' are permitted, among
	// other special characters)  Some examples we want to match:
	// * https://oidc.eks.us-west-2.amazonaws.com/id/B02C93B6A2D30341AD01E1B6D48164CB
	// * https://container.googleapis.com/v1/projects/mattmoor-credit/locations/us-west1-b/clusters/tenant-cluster
	// +optional
	// +listType=map
	// +listMapKey=issuer
	MetaIssuers []OIDCIssuer `json:"metaIssuers,omitempty"`

	// Metadata used for the CIProvider identity provider principal
	// +listType=map
	// +listMapKey=issuerName
	CIIssuerMetadata []CIIssuerMetadata `json:"ciIssuerMetadata,omitempty"`
}

type OIDCIssuer struct {
	// The expected issuer of an OIDC token
	IssuerURL string `json:"issuerURL,omitempty"`
	// The expected issuer of an OIDC token
	//+required
	//+kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`
	//+required
	//+kubebuilder:validation:MinLength=1
	ClientID string `json:"clientID"`
	// Used to determine the subject of the certificate and if additional
	// certificate values are needed
	//+required
	//+kubebuilder:validation:Enum=buildkite-job;email;github-workflow;codefresh-workflow;gitlab-pipeline;chainguard-identity;kubernetes;spiffe;uri;username;ci-provider
	Type string `json:"type"`
	// CIProvider is an optional configuration to map token claims to extensions for CI workflows
	CIProvider string `json:"ciProvider,omitempty"`
	// Optional, if the issuer is in a different claim in the OIDC token
	IssuerClaim string `json:"issuerClaim,omitempty"`
	// The domain that must be present in the subject for 'uri' issuer types
	// Also used to create an email for 'username' issuer types
	SubjectDomain string `json:"subjectDomain,omitempty"`
	// SPIFFETrustDomain specifies the trust domain that 'spiffe' issuer types
	// issue ID tokens for. Tokens with a different trust domain will be
	// rejected.
	SPIFFETrustDomain string `json:"spiffeTrustDomain,omitempty"`
	// Optional, the challenge claim expected for the issuer
	// Set if using a custom issuer
	ChallengeClaim string `json:"challengeClaim,omitempty"`
}

type CIIssuerMetadata struct {
	// Name of the issuer
	//+required
	//+kubebuilder:validation:MinLength=1
	IssuerName string `json:"issuerName"`
	// Defaults contains key-value pairs that can be used for filling the templates from ExtensionTemplates
	// If a key cannot be found on the token claims, the template will use the defaults
	DefaultTemplateValues map[string]string `json:"defaultTemplateValues,omitempty"`
	// ExtensionTemplates contains a mapping between certificate extension and token claim
	// Provide either strings following https://pkg.go.dev/text/template syntax,
	// e.g "{{ .url }}/{{ .repository }}"
	// or non-templated strings with token claim keys to be replaced,
	// e.g "job_workflow_sha"
	ExtensionTemplates Extensions `json:"extensionTemplates,omitempty"`
	// Template for the Subject Alternative Name extension
	// It's typically the same value as Build Signer URI
	SubjectAlternativeNameTemplate string `json:"subjectAlternativeNameTemplate,omitempty"`
}

// Extensions contains all custom x509 extensions defined by Fulcio
type Extensions struct {
	// Reference to specific build instructions that are responsible for signing.
	BuildSignerURI string `json:"buildSignerURI,omitempty"` // 1.3.6.1.4.1.57264.1.9

	// Immutable reference to the specific version of the build instructions that is responsible for signing.
	BuildSignerDigest string `json:"buildSignerDigest,omitempty"` // 1.3.6.1.4.1.57264.1.10

	// Specifies whether the build took place in platform-hosted cloud infrastructure or customer/self-hosted infrastructure.
	RunnerEnvironment string `json:"runnerEnvironment,omitempty"` // 1.3.6.1.4.1.57264.1.11

	// Source repository URL that the build was based on.
	SourceRepositoryURI string `json:"sourceRepositoryURI,omitempty"` // 1.3.6.1.4.1.57264.1.12

	// Immutable reference to a specific version of the source code that the build was based upon.
	SourceRepositoryDigest string `json:"sourceRepositoryDigest,omitempty"` // 1.3.6.1.4.1.57264.1.13

	// Source Repository Ref that the build run was based upon.
	SourceRepositoryRef string `json:"sourceRepositoryRef,omitempty"` // 1.3.6.1.4.1.57264.1.14

	// Immutable identifier for the source repository the workflow was based upon.
	SourceRepositoryIdentifier string `json:"sourceRepositoryIdentifier,omitempty"` // 1.3.6.1.4.1.57264.1.15

	// Source repository owner URL of the owner of the source repository that the build was based on.
	SourceRepositoryOwnerURI string `json:"sourceRepositoryOwnerURI,omitempty"` // 1.3.6.1.4.1.57264.1.16

	// Immutable identifier for the owner of the source repository that the workflow was based upon.
	SourceRepositoryOwnerIdentifier string `json:"sourceRepositoryOwnerIdentifier,omitempty"` // 1.3.6.1.4.1.57264.1.17

	// Build Config URL to the top-level/initiating build instructions.
	BuildConfigURI string `json:"buildConfigURI,omitempty"` // 1.3.6.1.4.1.57264.1.18

	// Immutable reference to the specific version of the top-level/initiating build instructions.
	BuildConfigDigest string `json:"buildConfigDigest,omitempty"` // 1.3.6.1.4.1.57264.1.19

	// Event or action that initiated the build.
	BuildTrigger string `json:"buildTrigger,omitempty"` // 1.3.6.1.4.1.57264.1.20

	// Run Invocation URL to uniquely identify the build execution.
	RunInvocationURI string `json:"runInvocationURI,omitempty"` // 1.3.6.1.4.1.57264.1.21

	// Source repository visibility at the time of signing the certificate.
	SourceRepositoryVisibilityAtSigning string `json:"sourceRepositoryVisibilityAtSigning,omitempty"` // 1.3.6.1.4.1.57264.1.22
}

type FulcioCertStatus struct {
	PrivateKeyRef         *SecretKeySelector `json:"privateKeyRef,omitempty"`
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`
	CARef                 *SecretKeySelector `json:"caRef,omitempty"`
}

// FulcioStatus defines the observed state of Fulcio
type FulcioStatus struct {
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Certificate     *FulcioCertStatus     `json:"certificate,omitempty"`
	PKCS11          *FulcioPKCS11Status   `json:"pkcs11,omitempty"`
	Url             string                `json:"url,omitempty"`
	// PEM-encoded certificate chain (trust bundle) resolved from the running Fulcio service API.
	// Contains the signing certificate followed by any intermediate and root CA certificates.
	// +optional
	CertificateChain string `json:"certificateChain,omitempty"`
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

func (i *Fulcio) GetServiceURL() string {
	return i.Status.Url
}
