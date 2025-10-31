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

// TASOperatorConfigSpec defines the desired state of TASOperatorConfig
type TASOperatorConfigSpec struct {
	// Platform type detected or configured (e.g., "openshift", "kubernetes")
	//+kubebuilder:validation:Enum=openshift;kubernetes
	Platform string `json:"platform,omitempty"`
}

// TASOperatorConfigStatus defines the observed state of TASOperatorConfig
type TASOperatorConfigStatus struct {
	// DetectionMethod indicates how the platform was determined
	//+kubebuilder:validation:Enum=auto-detected;command-line
	DetectionMethod string `json:"detectionMethod,omitempty"`

	// DetectionTimestamp records when the platform was detected
	DetectionTimestamp metav1.Time `json:"detectionTimestamp,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Platform",type=string,JSONPath=`.spec.platform`,description="The detected platform type"
//+kubebuilder:printcolumn:name="Detection Method",type=string,JSONPath=`.status.detectionMethod`,description="How the platform was detected"

// TASOperatorConfig is the Schema for the operatorconfigs API
type TASOperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TASOperatorConfigSpec   `json:"spec,omitempty"`
	Status TASOperatorConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TASOperatorConfigList contains a list of TASOperatorConfig
type TASOperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TASOperatorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TASOperatorConfig{}, &TASOperatorConfigList{})
}
