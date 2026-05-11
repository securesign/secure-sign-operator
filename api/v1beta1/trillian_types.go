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

type TrillianSpec struct {
	//+kubebuilder:default:={create: true, pvc: {size: "5Gi", retain: true, accessModes: {ReadWriteOnce}}}
	Db TrillianDB `json:"database,omitempty"`
	Monitoring MonitoringConfig `json:"monitoring,omitempty"`
	LogServer TrillianLogServer `json:"server,omitempty"`
	LogSigner TrillianLogSigner `json:"signer,omitempty"`
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
	//+kubebuilder:default:=153600
	//+optional
	MaxRecvMessageSize *int64 `json:"maxRecvMessageSize,omitempty"`
	//+optional
	Auth *Auth `json:"auth,omitempty"`
}

type trillianService struct {
	PodRequirements `json:",inline"`
	//+optional
	TLS TLS `json:"tls,omitempty"`
}

type TrillianLogServer trillianService

type TrillianLogSigner trillianService

type TrillianDB struct {
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Create *bool `json:"create"`
	//+optional
	// +kubebuilder:validation:Deprecated=true
	DatabaseSecretRef *LocalObjectReference `json:"databaseSecretRef,omitempty"`
	//+kubebuilder:default:={size: "5Gi", retain: true}
	Pvc Pvc `json:"pvc,omitempty"`
	//+optional
	TLS TLS `json:"tls,omitempty"`
	//+kubebuilder:validation:Enum={mysql, postgresql}
	//+kubebuilder:default:=mysql
	//+optional
	Provider string `json:"provider,omitempty"`
	//+kubebuilder:default:="$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)"
	//+optional
	Uri string `json:"uri,omitempty"`
}

type TrillianStatus struct {
	Db        TrillianDB        `json:"database,omitempty"`
	LogServer TrillianLogServer `json:"server,omitempty"`
	LogSigner TrillianLogSigner `json:"signer,omitempty"`
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

type Trillian struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrillianSpec   `json:"spec,omitempty"`
	Status TrillianStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type TrillianList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trillian `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trillian{}, &TrillianList{})
}

func (i *Trillian) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Trillian) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Trillian) GetTrustedCA() *LocalObjectReference {
	if i.Spec.TrustedCA != nil {
		return i.Spec.TrustedCA
	}
	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{Name: v}
	}
	return nil
}
