package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum:=external;internal
type TufSigningConfigURLMode string

const (
	SigningConfigURLExternal TufSigningConfigURLMode = "external"
	SigningConfigURLInternal TufSigningConfigURLMode = "internal"
)

type TufSpec struct {
	PodRequirements `json:",inline"`
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//+kubebuilder:default:=external
	//+optional
	SigningConfigURLMode TufSigningConfigURLMode `json:"signingConfigURLMode,omitempty"`
	//+kubebuilder:default:=80
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	Port int32 `json:"port,omitempty"`
	//+kubebuilder:default:={{name: rekor.pub},{name: ctfe.pub},{name: fulcio_v1.crt.pem},{name: tsa.certchain.pem}}
	//+kubebuilder:validation:MinItems:=1
	Keys []TufKey `json:"keys,omitempty"`
	//+kubebuilder:default:={name: tuf-root-keys}
	RootKeySecretRef *LocalObjectReference `json:"rootKeySecretRef,omitempty"`
	//+kubebuilder:default:={size: "100Mi",retain: true,accessModes: {ReadWriteOnce}}
	Pvc TufPvc `json:"pvc,omitempty"`
	//+kubebuilder:default:={prefix: trusted-artifact-signer}
	//+optional
	Ctlog CtlogService `json:"ctlog,omitempty"`
	//+optional
	Fulcio FulcioService `json:"fulcio,omitempty"`
	//+optional
	Rekor RekorService `json:"rekor,omitempty"`
	//+optional
	Tsa TsaService `json:"tsa,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.storageClass) || has(self.storageClass) && oldSelf.storageClass == self.storageClass)",message="storageClass is immutable when a PVC name is not specified"
// +kubebuilder:validation:XValidation:rule="oldSelf == null || has(self.name) || (!has(oldSelf.accessModes) || has(self.accessModes) && oldSelf.accessModes == self.accessModes)",message="accessModes is immutable when a PVC name is not specified"
type TufPvc struct {
	//+kubebuilder:default:="100Mi"
	Size *k8sresource.Quantity `json:"size,omitempty"`
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Retain *bool `json:"retain"`
	//+optional
	//+kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	//+kubebuilder:validation:MinLength=1
	//+kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`
	//+optional
	StorageClass string `json:"storageClass,omitempty"`
	//+kubebuilder:default:={ReadWriteOnce}
	//+kubebuilder:validation:MinItems:=1
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

type TufKey struct {
	//+required
	// +kubebuilder:validation:Enum:=rekor.pub;ctfe.pub;fulcio_v1.crt.pem;tsa.certchain.pem
	Name string `json:"name"`
	//+optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

type TufStatus struct {
	Keys    []TufKey `json:"keys,omitempty"`
	PvcName string   `json:"pvcName,omitempty"`
	Url     string   `json:"url,omitempty"`
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

// Tuf is the Schema for the tufs API
// +kubebuilder:validation:XValidation:rule="!(self.spec.replicas > 1) || ('ReadWriteMany' in self.spec.pvc.accessModes)",message="For deployments with more than 1 replica, pvc.accessModes must include 'ReadWriteMany'."
type Tuf struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TufSpec   `json:"spec,omitempty"`
	Status TufStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type TufList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tuf `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tuf{}, &TufList{})
}

func (i *Tuf) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Tuf) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Tuf) GetTrustedCA() *LocalObjectReference {
	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{Name: v}
	}
	return nil
}
