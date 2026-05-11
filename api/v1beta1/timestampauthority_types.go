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

package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:XValidation:rule=!(has(self.signer.certificateChain.certificateChainRef) && (has(self.signer.certificateChain.intermediateCA) || has(self.signer.certificateChain.leafCA) || has(self.signer.certificateChain.rootCA))),message="when certificateChainRef is set, intermediateCA, leafCA, and rootCA must not be set"
type TimestampAuthoritySpec struct {
	PodRequirements `json:",inline"`
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//+required
	Signer TimestampAuthoritySigner `json:"signer"`
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
	//+optional
	NTPMonitoring NTPMonitoring `json:"ntpMonitoring,omitempty"`
	//+kubebuilder:default:=1048576
	//+optional
	MaxRequestBodySize *int64 `json:"maxRequestBodySize,omitempty"`
}

// +kubebuilder:validation:XValidation:rule=(!(has(self.file) || has(self.kms) || has(self.tink)) || has(self.certificateChain.certificateChainRef)),message="signer config needs a matching cert chain in certificateChain.certificateChainRef"
// +kubebuilder:validation:XValidation:rule=(has(self.file) || has(self.kms) || has(self.tink) || !has(self.certificateChain.certificateChainRef)),message="certificateChainRef should not be present if no signers are configured"
// +kubebuilder:validation:XValidation:rule=(!(has(self.file) && has(self.kms)) && !(has(self.file) && has(self.tink)) && !(has(self.kms) && has(self.tink))),message="only one signer should be configured at any time"
type TimestampAuthoritySigner struct {
	//+required
	CertificateChain CertificateChain `json:"certificateChain"`
	//+optional
	File *File `json:"file,omitempty"`
	//+optional
	Kms *KMS `json:"kms,omitempty"`
	//+optional
	Tink *Tink `json:"tink,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(!has(self.rootCA) && !has(self.leafCA)) || (has(self.rootCA.privateKeyRef) == has(self.leafCA.privateKeyRef))",message="must provide private keys for both root and leaf certificate authorities"
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.rootCA.organizationName != ""),message=organizationName cannot be empty for root certificate authority
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.leafCA.organizationName != ""),message=organizationName cannot be empty for leaf certificate authority
// +kubebuilder:validation:XValidation:rule=(has(self.certificateChainRef) || self.intermediateCA[0].organizationName != ""),message="organizationName cannot be empty for intermediate certificate authority, please make sure all are in place"
type CertificateChain struct {
	//+optional
	CertificateChainRef *SecretKeySelector `json:"certificateChainRef,omitempty"`
	//+optional
	RootCA *TsaCertificateAuthority `json:"rootCA,omitempty"`
	//+optional
	IntermediateCA []*TsaCertificateAuthority `json:"intermediateCA,omitempty"`
	//+optional
	LeafCA *TsaCertificateAuthority `json:"leafCA,omitempty"`
}

type TsaCertificateAuthority struct {
	//+optional
	CommonName string `json:"commonName,omitempty"`
	//+optional
	OrganizationName string `json:"organizationName,omitempty"`
	//+optional
	OrganizationEmail string `json:"organizationEmail,omitempty"`
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
}

type File struct {
	//+optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	//+optional
	PrivateKeyRef *SecretKeySelector `json:"privateKeyRef,omitempty"`
}

type KMS struct {
	//+required
	KeyResource string `json:"keyResource,omitempty"`
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

type Tink struct {
	//+required
	KeyResource string `json:"keyResource,omitempty"`
	//+required
	KeysetRef *SecretKeySelector `json:"keysetRef,omitempty"`
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

type NTPMonitoring struct {
	//+kubebuilder:default:=true
	Enabled bool `json:"enabled"`
	Config *NtpMonitoringConfig `json:"config,omitempty"`
}

type NtpMonitoringConfig struct {
	NtpConfigRef    *LocalObjectReference `json:"ntpConfigRef,omitempty"`
	RequestAttempts int                   `json:"requestAttempts,omitempty"`
	RequestTimeout  int                   `json:"requestTimeout,omitempty"`
	NumServers      int                   `json:"numServers,omitempty"`
	MaxTimeDelta    int                   `json:"maxTimeDelta,omitempty"`
	ServerThreshold int                   `json:"serverThreshold,omitempty"`
	Period          int                   `json:"period,omitempty"`
	Servers         []string              `json:"servers,omitempty"`
}

func (i *TimestampAuthority) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *TimestampAuthority) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

type TimestampAuthorityStatus struct {
	NTPMonitoring *NTPMonitoring            `json:"ntpMonitoring,omitempty"`
	Signer        *TimestampAuthoritySigner `json:"signer,omitempty"`
	Url           string                    `json:"url,omitempty"`
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

type TimestampAuthority struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TimestampAuthoritySpec   `json:"spec,omitempty"`
	Status TimestampAuthorityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type TimestampAuthorityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TimestampAuthority `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TimestampAuthority{}, &TimestampAuthorityList{})
}

func (i *TimestampAuthority) GetTrustedCA() *LocalObjectReference {
	if i.Spec.TrustedCA != nil {
		return i.Spec.TrustedCA
	}
	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{Name: v}
	}
	return nil
}
