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
}

// TimestampAuthorityStatus defines the observed state of TimestampAuthority
type TimestampAuthorityStatus struct {
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
