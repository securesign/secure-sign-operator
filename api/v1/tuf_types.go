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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TufSigningConfigURLMode controls which URLs are used in the TUF signing config.
// +kubebuilder:validation:Enum:=external;internal
type TufSigningConfigURLMode string

const (
	SigningConfigURLExternal TufSigningConfigURLMode = "external"
	SigningConfigURLInternal TufSigningConfigURLMode = "internal"
)

// TufSpec defines the desired state of Tuf
type TufSpec struct {
	PodRequirements      `json:",inline"`
	ServiceAccountConfig `json:",inline"`
	// SigningConfigURLMode is deprecated. URL mode is now autoresolved from Component Custom Resources.
	//+kubebuilder:default:=external
	// +kubebuilder:validation:Deprecated=true
	SigningConfigURLMode TufSigningConfigURLMode `json:"signingConfigURLMode,omitempty"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//+kubebuilder:default:=80
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	Port int32 `json:"port,omitempty"`
	// List of TUF targets which will be added to TUF root
	//+kubebuilder:default:={{name: rekor.pub},{name: ctfe.pub},{name: fulcio_v1.crt.pem},{name: tsa.certchain.pem}}
	//+kubebuilder:validation:MinItems:=1
	Keys []TufKey `json:"keys,omitempty"`
	// Secret object reference that will hold you repository root keys. This parameter will be used only with operator-managed repository.
	//+kubebuilder:default:={name: tuf-root-keys}
	RootKeySecretRef *LocalObjectReference `json:"rootKeySecretRef,omitempty"`
	// Pvc configuration of the persistent storage claim for deployment in the cluster.
	// You can use ReadWriteOnce accessMode if you don't have suitable storage provider but your deployment will not support HA mode
	//+kubebuilder:default:={size: "100Mi",retain: true,accessModes: {ReadWriteOnce}}
	Pvc TufPvc `json:"pvc,omitempty"`
	// Ctlog service configuration
	//+kubebuilder:default:={prefix: trusted-artifact-signer}
	//+optional
	Ctlog CtlogService `json:"ctlog,omitempty"`
	// Fulcio service configuration
	//+optional
	Fulcio FulcioService `json:"fulcio,omitempty"`
	// Rekor service configuration
	//+optional
	Rekor RekorService `json:"rekor,omitempty"`
	// TSA service configuration
	//+optional
	Tsa TsaService `json:"tsa,omitempty"`

	// ConfigMap with additional bundle of trusted CA
	// +optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
}

// TufPvc configuration of the persistent storage claim for deployment in the cluster.
// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.storageClass) || has(self.storageClass) && oldSelf.storageClass == self.storageClass)",message="storageClass is immutable when a PVC name is not specified"
// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.accessModes) || has(self.accessModes) && oldSelf.accessModes == self.accessModes)",message="accessModes is immutable when a PVC name is not specified"
type TufPvc struct {
	// The requested size of the persistent volume attached to Pod.
	// The format of this field matches that defined by kubernetes/apimachinery.
	// See https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity for more info on the format of this field.
	//+kubebuilder:default:="100Mi"
	Size *k8sresource.Quantity `json:"size,omitempty"`

	// Retain policy for the PVC
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Retain *bool `json:"retain"`
	// Name of the PVC
	//+optional
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`
	// The name of the StorageClass to claim a PersistentVolume from.
	//+optional
	StorageClass string `json:"storageClass,omitempty"`
	// PersistentVolume AccessModes. Configure ReadWriteMany for HA deployment.
	//+kubebuilder:default:={ReadWriteOnce}
	//+kubebuilder:validation:MinItems:=1
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

type TufKey struct {
	// File name which will be used as TUF target.
	//+required
	// +kubebuilder:validation:Enum:=rekor.pub;ctfe.pub;fulcio_v1.crt.pem;tsa.certchain.pem
	Name string `json:"name"`
	// Reference to secret object
	// If it is unset, the operator will try to autoconfigure secret reference, by searching secrets in namespace which
	// contain `rhtas.redhat.com/$name` label.
	//+optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

type TufKeyStatus struct {
	Name      string             `json:"name"`
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

func (s TufKeyStatus) MatchesSpec(spec TufKey) bool {
	return spec.Name == s.Name &&
		equality.Semantic.DeepDerivative(spec.SecretRef, s.SecretRef)
}

// TufStatus defines the observed state of Tuf
type TufStatus struct {
	Keys    []TufKeyStatus `json:"keys,omitempty"`
	PvcName string         `json:"pvcName,omitempty"`
	Url     string         `json:"url,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

func (s TufStatus) MatchesKeys(specKeys []TufKey) bool {
	if len(specKeys) != len(s.Keys) {
		return false
	}
	for i := range specKeys {
		if !s.Keys[i].MatchesSpec(specKeys[i]) {
			return false
		}
	}
	return true
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The component status"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

// Tuf is the Schema for the tufs API
// +kubebuilder:validation:XValidation:rule="!(self.spec.replicas > 1) || ('ReadWriteMany' in self.spec.pvc.accessModes)",message="For deployments with more than 1 replica, pvc.accessModes must include 'ReadWriteMany'."
type Tuf struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TufSpec   `json:"spec,omitempty"`
	Status TufStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TufList contains a list of Tuf
type TufList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tuf `json:"items"`
}

func (i *Tuf) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Tuf) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Tuf) GetTrustedCA() *LocalObjectReference {
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

func (i *Tuf) GetServiceURL() string {
	return i.Status.Url
}
