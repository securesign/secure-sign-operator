/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CTlogSignerType specifies how the CTlog signing key is managed.
// +kubebuilder:validation:Enum=file;pkcs11
type CTlogSignerType string

const (
	CTlogSignerTypeFile   CTlogSignerType = "file"
	CTlogSignerTypePKCS11 CTlogSignerType = "pkcs11"
)

// CTlogSpec defines the desired state of CTlog component
// +kubebuilder:validation:XValidation:rule=(!has(self.publicKeyRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.privateKeyPasswordRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.signerType) || self.signerType != 'pkcs11' || has(self.pkcs11)),message=pkcs11 config is required when signerType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.signerType) || self.signerType != 'file' || !has(self.pkcs11)),message=pkcs11 config must not be set when signerType is file
// +kubebuilder:validation:XValidation:rule=(!has(self.signerType) || self.signerType != 'pkcs11' || !has(self.privateKeyRef)),message=privateKeyRef must not be set when signerType is pkcs11
// +kubebuilder:validation:XValidation:rule=(!has(self.signerType) || self.signerType != 'pkcs11' || !has(self.privateKeyPasswordRef)),message=privateKeyPasswordRef must not be set when signerType is pkcs11
type CTlogSpec struct {
	PodRequirements      `json:",inline"`
	ServiceAccountConfig `json:",inline"`

	// SignerType selects the key management backend: "file" (default) or "pkcs11".
	//+optional
	//+kubebuilder:default:="file"
	SignerType CTlogSignerType `json:"signerType,omitempty"`

	// PKCS11 configuration. Required when signerType is "pkcs11".
	//+optional
	PKCS11 *CTlogPKCS11Config `json:"pkcs11,omitempty"`

	// The ID of a Trillian tree that stores the log data.
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
	//+kubebuilder:validation:Minimum=1
	TreeID *int64 `json:"treeID,omitempty"`

	// The private key used for signing STHs etc.
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`

	// Deprecated: Legacy PEM encryption as specified in RFC 1423 is insecure by design
	// and not FIPS-compliant. Auto-generated keys are no longer password-encrypted;
	// this field is retained only for backward compatibility with existing user-provided
	// encrypted keys. Kubernetes Secrets provide encryption-at-rest.
	// +optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// The public key matching the private key (if both are present). It is
	// used only by mirror logs for verifying the source log's signatures, but can
	// be specified for regular logs as well for the convenience of test tools.
	//+optional
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`

	// List of secrets containing root certificates that are acceptable to the log.
	// The certs are served through get-roots endpoint. Optional in mirrors.
	//+optional
	// +listType=atomic
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`

	//Enable Service monitors for ctlog
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`

	// Trillian service configuration
	Trillian TrillianService `json:"trillian,omitempty"`

	// Secret holding Certificate Transparency server config in text proto format
	// If it is set then any setting of treeID, privateKeyRef, privateKeyPasswordRef,
	// publicKeyRef, rootCertificates and trillian will be overridden.
	//+optional
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`

	// Prefix is the name of the log. The prefix cannot be empty and can
	// contain "/" path separator characters to define global override handler prefix.
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9/]*[a-z0-9])?$"
	//+optional
	Prefix string `json:"prefix,omitempty"`

	// Configuration for enabling TLS (Transport Layer Security) encryption for manged service.
	//+optional
	TLS TLS `json:"tls,omitempty"`

	// Max certificate chain size in bytes. Passed as --max_cert_chain_size.
	//+optional
	//+kubebuilder:validation:Minimum=1
	MaxCertChainSize *int64 `json:"maxCertChainSize,omitempty"`

	// ConfigMap with additional bundle of trusted CA
	// +optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

// CTlogPKCS11Config configures CTlog to use a PKCS#11 backend for signing.
// The operator is vendor-agnostic: the init containers provision the token
// and make the .so library available.
//
// +kubebuilder:validation:XValidation:rule="has(self.pinSecretRef) && has(self.publicKeyRef)",message="pinSecretRef and publicKeyRef are required"
// +kubebuilder:validation:XValidation:rule="has(self.initContainers) && size(self.initContainers) > 0",message="at least one initContainer is required to provision the PKCS#11 library"
type CTlogPKCS11Config struct {
	// Init containers that run before the CTlog server for vendor-specific HSM initialization.
	// At least one is required to make the PKCS#11 .so library available.
	//+required
	InitContainers []PKCS11InitContainerSpec `json:"initContainers"`

	// Additional pod-level volumes needed for HSM connectivity.
	//+optional
	Volumes []core.Volume `json:"volumes,omitempty"`

	// Reference to a Secret containing the HSM PIN.
	//+required
	PinSecretRef *SecretKeySelector `json:"pinSecretRef,omitempty"`

	// Reference to a Secret containing the PEM-encoded public key
	// corresponding to the private key on the HSM.
	//+required
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`

	// PKCS#11 token label (e.g. "ctlog").
	//+required
	TokenLabel string `json:"tokenLabel"`

	// Full path to the PKCS#11 .so module inside the init container image.
	// The operator copies this library to a shared volume and passes
	// --pkcs11_module_path to ct_server.
	//+required
	PKCS11ModulePath string `json:"pkcs11ModulePath"`

	// Additional environment variables for the main CTlog server container.
	// Use for vendor-specific env vars (e.g. SOFTHSM2_CONF).
	//+optional
	ServerEnv []core.EnvVar `json:"serverEnv,omitempty"`

	// Additional volume mounts for the CTlog server container.
	//+optional
	ServerVolumeMounts []core.VolumeMount `json:"serverVolumeMounts,omitempty"`

	// Persistent storage for HSM tokens (key survives pod restarts).
	// When nil, an emptyDir is used (key is regenerated on pod restart).
	//+optional
	Persistence *Pvc `json:"persistence,omitempty"`
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	ServerConfigRef       *LocalObjectReference `json:"serverConfigRef,omitempty"`
	PrivateKeyRef         *SecretKeySelector    `json:"privateKeyRef,omitempty"`
	PrivateKeyPasswordRef *SecretKeySelector    `json:"privateKeyPasswordRef,omitempty"`
	PublicKeyRef          *SecretKeySelector    `json:"publicKeyRef,omitempty"`
	// +listType=atomic
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`
	// PEM-encoded public key resolved from the CTlog signer secret.
	// +optional
	PublicKey string `json:"publicKey,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	TreeID *int64 `json:"treeID,omitempty"`
	// PKCS11 configuration resolved from spec. Populated by ensure-pkcs11-config action.
	//+optional
	PKCS11 *CTlogPKCS11Status `json:"pkcs11,omitempty"`
	// Configuration for enabling TLS (Transport Layer Security) encryption for manged service.
	//+optional
	TLS TLS `json:"tls,omitempty"`
	// Url is the CTlog endpoint URL including the log prefix path,
	// e.g. http://ctlog.namespace.svc/trusted-artifact-signer.
	Url string `json:"url,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// CTlogPKCS11Status holds the resolved PKCS#11 references for CTlog.
type CTlogPKCS11Status struct {
	// Resolved reference to the HSM PIN secret.
	PinSecretRef *SecretKeySelector `json:"pinSecretRef,omitempty"`
	// Resolved reference to the PEM public key secret.
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`
	// Token label for the PKCS#11 token.
	TokenLabel string `json:"tokenLabel,omitempty"`
	// Module path for the PKCS#11 .so.
	PKCS11ModulePath string `json:"pkcs11ModulePath,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The component status"

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

func (i *CTlog) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *CTlog) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *CTlog) GetTrustedCA() *LocalObjectReference {
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

func (i *CTlog) GetServiceURL() string {
	return i.Status.Url
}
