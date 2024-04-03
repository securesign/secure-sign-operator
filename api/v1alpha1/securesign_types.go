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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecuresignSpec defines the desired state of Securesign
type SecuresignSpec struct {
	Rekor    RekorSpec    `json:"rekor,omitempty"`
	Fulcio   FulcioSpec   `json:"fulcio,omitempty"`
	Trillian TrillianSpec `json:"trillian,omitempty"`
	// If true, the Operator will create segment backup job and cronjob and required RBAC
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=false
	Analytics bool `json:"analytics"`
	//+kubebuilder:default:={keys:{{name: rekor.pub},{name: ctfe.pub},{name: fulcio_v1.crt.pem}}}
	Tuf   TufSpec   `json:"tuf,omitempty"`
	Ctlog CTlogSpec `json:"ctlog,omitempty"`
}

// SecuresignStatus defines the observed state of Securesign
type SecuresignStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions   []metav1.Condition     `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	RekorStatus  SecuresignRekorStatus  `json:"rekor,omitempty"`
	FulcioStatus SecuresignFulcioStatus `json:"fulcio,omitempty"`
	TufStatus    SecuresignTufStatus    `json:"tuf,omitempty"`
}

type SecuresignRekorStatus struct {
	Url string `json:"url,omitempty"`
}

type SecuresignFulcioStatus struct {
	Url string `json:"url,omitempty"`
}

type SecuresignTufStatus struct {
	Url string `json:"url,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The Deployment status"
//+kubebuilder:printcolumn:name="Rekor URL",type=string,JSONPath=`.status.rekor.url`,description="The rekor url"
//+kubebuilder:printcolumn:name="Fulcio URL",type=string,JSONPath=`.status.fulcio.url`,description="The fulcio url"
//+kubebuilder:printcolumn:name="Tuf URL",type=string,JSONPath=`.status.tuf.url`,description="The tuf url"

// Securesign is the Schema for the securesigns API
type Securesign struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecuresignSpec   `json:"spec,omitempty"`
	Status SecuresignStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecuresignList contains a list of Securesign
type SecuresignList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Securesign `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Securesign{}, &SecuresignList{})
}
