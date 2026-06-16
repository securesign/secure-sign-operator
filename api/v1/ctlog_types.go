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
// +kubebuilder:validation:XValidation:rule=(!has(self.publicKeyRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.privateKeyPasswordRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type CTlogSpec struct {
	PodRequirements `json:",inline"`
	// The ID of a Trillian tree that stores the log data.
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
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
	RootCertificates []SecretKeySelector `json:"rootCertificates,omitempty"`

	//Enable Service monitors for ctlog
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`

	// Trillian service configuration
	//+kubebuilder:default:={port: 8091}
	Trillian TrillianService `json:"trillian,omitempty"`

	// Secret holding Certificate Transparency server config in text proto format
	// If it is set then any setting of treeID, privateKeyRef, privateKeyPasswordRef,
	// publicKeyRef, rootCertificates and trillian will be overridden.
	//+optional
	ServerConfigRef *LocalObjectReference `json:"serverConfigRef,omitempty"`

	// Configuration for enabling TLS (Transport Layer Security) encryption for manged service.
	//+optional
	TLS TLS `json:"tls,omitempty"`

	// Max certificate chain size in bytes. Passed as --max_cert_chain_size.
	//+kubebuilder:default:=153600
	//+optional
	MaxCertChainSize *int64 `json:"maxCertChainSize,omitempty"`
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	ServerConfigRef       *LocalObjectReference `json:"serverConfigRef,omitempty"`
	PrivateKeyRef         *SecretKeySelector    `json:"privateKeyRef,omitempty"`
	PrivateKeyPasswordRef *SecretKeySelector    `json:"privateKeyPasswordRef,omitempty"`
	PublicKeyRef          *SecretKeySelector    `json:"publicKeyRef,omitempty"`
	RootCertificates      []SecretKeySelector   `json:"rootCertificates,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	// +kubebuilder:validation:Type=number
	TreeID *int64 `json:"treeID,omitempty"`
	// Configuration for enabling TLS (Transport Layer Security) encryption for manged service.
	//+optional
	TLS TLS    `json:"tls,omitempty"`
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

func (i *CTlog) GetTrustedCA() *LocalObjectReference {
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
