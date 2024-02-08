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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrillianSpec defines the desired state of Trillian
type TrillianSpec struct {
	// Define your database connection
	Db TrillianDB `json:"database,omitempty"`
}

type TrillianDB struct {
	// Create Database if a database is not created one must be defined using the DatabaseSecret field
	// default: true
	Create bool `json:"create,omitempty"`
	// PVC configuration
	Pvc TrillianPvc `json:"pvc,omitempty"`
	// Secret with values to be used to connect to an existing DB or to be used with the creation of a new DB
	DatabaseSecretRef *v1.LocalObjectReference `json:"databaseSecretRef,omitempty"`
}

type TrillianPvc struct {
	// Retain the PVC after Trillian is deleted
	Retain bool `json:"retain,omitempty"`
	// PVC size for Trillian
	//+kubebuilder:default:="5Gi"
	Size string `json:"size,omitempty"`
	// PVC name
	//+kubebuilder:default:=trillian-mysql
	Name string `json:"name,omitempty"`
}

// TrillianStatus defines the observed state of Trillian
type TrillianStatus struct {
	Phase Phase `json:"phase"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The component phase"

// Trillian is the Schema for the trillians API
type Trillian struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrillianSpec   `json:"spec,omitempty"`
	Status TrillianStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TrillianList contains a list of Trillian
type TrillianList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trillian `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trillian{}, &TrillianList{})
}
