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
// +kubebuilder:validation:XValidation:rule="!has(self.logs) || self.logs.filter(x, has(x.active) && x.active == true).size() == 1",message="exactly one log should be active"
type CTlogSpec struct {
	PodRequirements      `json:",inline"`
	ServiceAccountConfig `json:",inline"`

	// Logs defines the list of certificate transparency logs (active and frozen shards).
	// Each entry represents either the active log or a frozen shard.
	// +optional
	// +listType=map
	// +listMapKey=prefix
	// +patchStrategy=merge
	// +patchMergeKey=prefix
	Logs []CTLogConfig `json:"logs,omitempty" patchStrategy:"merge" patchMergeKey:"prefix"`

	// Define whether you want to export service or not
	Ingress Ingress `json:"ingress,omitempty"`

	//Enable Service monitors for ctlog
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`

	// Trillian service configuration
	Trillian ServiceReference `json:"trillian,omitempty"`

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

// CTLogConfig defines the configuration for a certificate transparency log (active or frozen).
// +structType=atomic
// +kubebuilder:validation:XValidation:rule="!has(self.signer) || !has(self.signer.file) || !has(self.signer.file.publicKeyRef) || has(self.signer.file.privateKeyRef) || (has(self.readonly) && self.readonly == true)",message="privateKeyRef cannot be empty for non-readonly logs when publicKeyRef is set"
type CTLogConfig struct {
	// LogId is the Trillian tree ID. For the active log, the operator will
	// generate one if not set. For frozen/readonly shards, this must be the
	// existing tree ID from the original active log.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	LogId *int64 `json:"logId,omitempty"`

	// Prefix is the name of the log. The prefix cannot be empty and can
	// contain "/" path separator characters to define global override handler prefix.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9/]*[a-z0-9])?$"
	Prefix string `json:"prefix"`

	// Roots is a list of secrets containing root certificates acceptable to the log.
	// The certs are served through get-roots endpoint. Optional for mirrors.
	// +optional
	// +listType=atomic
	Roots []SecretKeySelector `json:"roots,omitempty"`

	// Signer configuration. Required for active and frozen logs. Optional only for mirrors.
	// +optional
	Signer *CTlogSigner `json:"signer,omitempty"`

	// RFC3339 timestamp when this log's certificates become valid.
	// +optional
	NotAfterStart *metav1.Time `json:"notAfterStart,omitempty"`

	// RFC3339 timestamp when this log's certificates expire.
	// +optional
	NotAfterLimit *metav1.Time `json:"notAfterLimit,omitempty"`

	// Mirror indicates if this is a mirror log (read-only, no signing).
	// +optional
	Mirror *bool `json:"mirror,omitempty"`

	// Readonly indicates if this is a frozen/read-only shard.
	// +optional
	Readonly *bool `json:"readonly,omitempty"`

	// FrozenSTH is the frozen SignedTreeHead for a frozen shard.
	// Only meaningful when readonly is true.
	// +optional
	FrozenSTH *CTLogFrozenSTH `json:"frozenSTH,omitempty"`

	// Active indicates if this is the currently active log. Only one log should be active at a time.
	// +optional
	Active *bool `json:"active,omitempty"`
}

// CTLogFrozenSTH represents a frozen SignedTreeHead for a read-only shard.
// +structType=atomic
type CTLogFrozenSTH struct {
	// TreeSize is the number of leaves in the tree.
	// +kubebuilder:validation:Minimum=0
	TreeSize *int64 `json:"treeSize,omitempty"`

	// Timestamp is the Unix timestamp when the STH was signed.
	// +optional
	Timestamp *metav1.Time `json:"timestamp,omitempty"`

	// Sha256RootHash is the Base64-encoded root hash.
	// +optional
	Sha256RootHash []byte `json:"sha256RootHash,omitempty"`

	// TreeHeadSignature is the Base64-encoded signature.
	// +optional
	TreeHeadSignature []byte `json:"treeHeadSignature,omitempty"`
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

// CTlogLogStatus contains status information for a single log.
// +structType=atomic
type CTlogLogStatus struct {
	// LogId is the Trillian tree ID.
	LogId *int64 `json:"logId,omitempty"`
	// Prefix is the log's URL prefix.
	// +kubebuilder:validation:Required
	Prefix string `json:"prefix,omitempty"`
	// PrivateKeyRef points to the secret containing the private key (for active logs only).
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// PrivateKeyPasswordRef points to the secret containing the private key password (if encrypted).
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`
	// PublicKeyRef points to the secret containing the public key.
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`
	// PublicKey is the PEM-encoded public key.
	PublicKey string `json:"publicKey,omitempty"`
	// RootCertificates are the resolved root certificates.
	// +listType=atomic
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`
	// Readonly indicates if this is a frozen shard.
	Readonly *bool `json:"readonly,omitempty"`
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`
	// Logs contains status information for each log.
	// +listType=map
	// +listMapKey=prefix
	// +patchStrategy=merge
	// +patchMergeKey=prefix
	// +optional
	Logs []CTlogLogStatus `json:"logs,omitempty"`

	// Deprecated: Use logs[0].PrivateKeyRef instead
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
	// Deprecated: Use logs[0].PrivateKeyPasswordRef instead
	PrivateKeyPasswordRef *SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`
	// Deprecated: Use logs[0].PublicKeyRef instead
	PublicKeyRef *SecretKeySelector `json:"publicKeyRef,omitempty"`
	// Deprecated: Use logs[0].RootCertificates instead
	// +listType=atomic
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`
	// PEM-encoded public key resolved from the CTlog signer secret.
	// Deprecated: Use logs[0].PublicKey instead
	// +optional
	PublicKey string `json:"publicKey,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	// Deprecated: Use logs[0].LogId instead
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
