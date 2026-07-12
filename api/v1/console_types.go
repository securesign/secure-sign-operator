/*
Copyright 2026.

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

// ConsoleSpec defines the desired state of the Console
type ConsoleSpec struct {
	ServiceAccountConfig `json:",inline"`
	// Configuration for Console UI service
	UI ConsoleUI `json:"ui,omitempty"`
	// Configuration for Console Api service
	Api ConsoleAPI `json:"api,omitempty"`

	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

type ConsoleUI struct {
	PodRequirements `json:",inline"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
}

type ConsoleAPI struct {
	PodRequirements `json:",inline"`
	// TUF service configuration
	//+optional
	Tuf TufService `json:"tuf,omitempty"`
	// Configuration for enabling TLS (Transport Layer Security) encryption for manged service.
	//+optional
	TLS TLS `json:"tls,omitempty"`
}

type ConsoleAPIStatus struct {
	TLS TLS `json:"tls,omitempty"`
}

type ConsoleUIStatus struct {
	Url string `json:"url,omitempty"`
}

// ConsoleStatus defines the observed state of the Console
type ConsoleStatus struct {
	Api ConsoleAPIStatus `json:"api,omitempty"`
	UI  ConsoleUIStatus  `json:"ui,omitempty"`
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

// Console is the Schema for the consoles API
type Console struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsoleSpec   `json:"spec,omitempty"`
	Status ConsoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConsoleList contains a list of the Console
type ConsoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Console `json:"items"`
}

func (i *Console) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Console) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Console) GetTrustedCA() *LocalObjectReference {
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
