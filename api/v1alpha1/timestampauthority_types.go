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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TimestampAuthoritySpec defines the desired state of TimestampAuthority
type TimestampAuthoritySpec struct {
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	// Signer configuration
	Signer TimestampAuthoritySigner `json:"signer,omitempty"`
}

type TimestampAuthoritySigner struct {
	// Timestamping authority signer. Valid options include: [kms, tink, file].
	Type string `json:"type,omitempty"`
	// Configuration for the Certificate Chain
	CertificateChain CertificateChain `json:"certificateChain,omitempty"`
	// Configuration for file-based signer
	//+optional
	FileSigner FileSigner `json:"fileSigner,omitempty"`
}

type CertificateChain struct {
	// CommonName specifies the common name for the TimeStampAuthorities cert chain.
	// If not provided, the common name will default to the host name.
	//+optional
	CommonName string `json:"commonName,omitempty"`
	//+optional
	//OrganizationName specifies the Organization Name for the TimeStampAuthorities cert chain.
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	//Organization Email specifies the Organization Email for the TimeStampAuthorities cert chain.
	OrganizationEmail string `json:"organizationEmail,omitempty"`
	//Reference to the certificate chain
	//+optional
	CertificateChainRef *SecretKeySelector `json:"certificateChainRef,omitempty"`
	// Password to decrypt the signer's root private key
	//+optional
	RootPasswordRef *SecretKeySelector `json:"rootPasswordRef,omitempty"`
	// Reference to the signer's root private key
	//+optional
	RootPrivateKeyRef *SecretKeySelector `json:"rootPrivateKeyRef,omitempty"`
	// Password to decrypt the signer's Intermediate private key
	//+optional
	InterPasswordRef *SecretKeySelector `json:"interPasswordRef,omitempty"`
	// Reference to the signer's Intermediate private key
	//+optional
	InterPrivateKeyRef *SecretKeySelector `json:"interPrivateKeyRef,omitempty"`
}

type FileSigner struct {
	// Password to decrypt the signer's root private key
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	// Reference to the signer's root private key
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
}

// TimestampAuthorityStatus defines the observed state of TimestampAuthority
type TimestampAuthorityStatus struct {
	Signer *TimestampAuthoritySigner `json:"signer,omitempty"`
	Url    string                    `json:"url,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// TimestampAuthority is the Schema for the timestampauthorities API
type TimestampAuthority struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TimestampAuthoritySpec   `json:"spec,omitempty"`
	Status TimestampAuthorityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TimestampAuthorityList contains a list of TimestampAuthority
type TimestampAuthorityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TimestampAuthority `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TimestampAuthority{}, &TimestampAuthorityList{})
}
