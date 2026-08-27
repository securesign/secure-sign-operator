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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CTlogSpec defines the desired state of CTlog component
type CTlogSpec struct {
	PodRequirements      `json:",inline"`
	ServiceAccountConfig `json:",inline"`
	// The ID of a Trillian tree that stores the log data.
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
	//+kubebuilder:validation:Minimum=1
	TreeID *int64 `json:"treeID,omitempty"`

	// Signer configuration
	//+required
	Signer CTlogSigner `json:"signer"`

	// List of secrets containing root certificates that are acceptable to the log.
	// The certs are served through get-roots endpoint. Optional in mirrors.
	//+optional
	// +listType=atomic
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`

	// Define whether you want to export service or not
	Ingress Ingress `json:"ingress,omitempty"`

	//Enable Service monitors for ctlog
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`

	// Trillian service configuration
	Trillian ServiceReference `json:"trillian,omitempty"`

	// Inactive shards
	// +listType=map
	// +listMapKey=treeID
	// +patchStrategy=merge
	// +patchMergeKey=treeID
	Sharding []CTlogLogRange `json:"sharding,omitempty"`

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
	TrustedCA     *LocalObjectReference `json:"trustedCA,omitempty"`
	PodExtensions `json:",inline"`
	// Authentication configuration for the signer backend.
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

// CTlogPKCS11Config holds the CTLog PKCS#11/HSM signer configuration.
// HSM token persistence (e.g. SoftHSM PVC) should be configured through
// spec.ctlog.volumes rather than through this struct.
// +kubebuilder:validation:XValidation:rule="has(self.publicKeyRef)",message="publicKeyRef is required for CTLog PKCS#11 signer"
// +kubebuilder:validation:XValidation:rule="has(self.modulePath)",message="modulePath is required for CTLog PKCS#11 signer"
// +kubebuilder:validation:XValidation:rule="has(self.tokenLabel)",message="tokenLabel is required for CTLog PKCS#11 signer"
// +kubebuilder:validation:XValidation:rule="has(self.pinSecretRef)",message="pinSecretRef is required for CTLog PKCS#11 signer"
type CTlogPKCS11Config struct {
	// Absolute path to the PKCS#11 module (.so).
	//+required
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:Pattern=`^/.+\..+$`
	ModulePath string `json:"modulePath,omitempty"`
	// Token label identifying the HSM slot.
	//+required
	//+kubebuilder:validation:MinLength=1
	TokenLabel string `json:"tokenLabel,omitempty"`
	// Reference to a Secret key containing the HSM user PIN.
	//+required
	PinSecretRef *SecretKeySelector `json:"pinSecretRef,omitempty"`
	// PEM-encoded public key matching the HSM-resident private key.
	//+required
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef"`
}

// CTlogLogRange defines the range and key details of an inactive CTlog shard
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'file' || has(self.privateKeyRef)",message="privateKeyRef is required for file-type shards"
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'pkcs11' || has(self.privateKeyRef)",message="privateKeyRef is required for pkcs11-type shards"
// +structType=atomic
type CTlogLogRange struct {
	// ID of Merkle tree in Trillian backend
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	TreeID int64 `json:"treeID"`
	// Type of signer backend for this shard (file or pkcs11)
	// +kubebuilder:validation:Enum=file;pkcs11
	// +optional
	Type string `json:"type,omitempty"`
	// Prefix for the shard's URL path (e.g., "shard-12345"). Required to generate proper shard URLs.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9/]*[a-z0-9])?$"
	Prefix string `json:"prefix"`
	// Reference to a secret containing the public key for the log shard
	// +kubebuilder:validation:Optional
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`
	// Reference to a secret containing the private key for the log shard
	// +kubebuilder:validation:Required
	PrivateKeyRef SecretKeySelector `json:"privateKeyRef"`
	// Reference to a secret containing the password for the private key (if encrypted).
	// Passwords are not FIPS-compliant. Kept for backward compatibility with legacy instances.
	// +optional
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`
	// RFC3339 timestamp when this shard's certificates become valid.
	// Example: "2024-01-01T00:00:00Z"
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z$"
	NotAfterStart *string `json:"notAfterStart,omitempty"`
	// RFC3339 timestamp when this shard's certificates expire.
	// Example: "2025-01-01T00:00:00Z"
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z$"
	NotAfterLimit *string `json:"notAfterLimit,omitempty"`
}

// CTlogSigner defines the desired state of the CTlog Signer
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'pkcs11' || has(self.pkcs11)",message="pkcs11 configuration is required when type is pkcs11"
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'pkcs11' || !has(self.file)",message="file configuration must not be set when type is pkcs11"
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'file' || !has(self.pkcs11)",message="pkcs11 configuration must not be set when type is file"
type CTlogSigner struct {
	// Type of the signer backend
	//+kubebuilder:validation:Enum=file;pkcs11
	//+optional
	Type string `json:"type,omitempty"`
	// Configuration for file-based signer
	//+optional
	File *CTlogFile `json:"file,omitempty"`
	// Configuration for PKCS#11/HSM-based signer
	//+optional
	PKCS11 *CTlogPKCS11Config `json:"pkcs11,omitempty"`
}

// CTlogFile defines the desired state of the CTlog file-based signer
// +kubebuilder:validation:XValidation:rule=(!has(self.publicKeyRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type CTlogFile struct {
	// The private key used for signing STHs etc.
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`

	// The public key matching the private key (if both are present). It is
	// used only by mirror logs for verifying the source log's signatures, but can
	// be specified for regular logs as well for the convenience of test tools.
	//+optional
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`
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

func (i *CTlog) RemoveCondition(conditionType string) {
	meta.RemoveStatusCondition(&i.Status.Conditions, conditionType)
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
