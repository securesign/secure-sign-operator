package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TufSpec defines the desired state of Tuf
type TufSpec struct {
	PodRequirements `json:",inline"`
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
}

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

// TufStatus defines the observed state of Tuf
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

func init() {
	SchemeBuilder.Register(&Tuf{}, &TufList{})
}

func (i *Tuf) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Tuf) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}
