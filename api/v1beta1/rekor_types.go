package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RekorSpec defines the desired state of Rekor
type RekorSpec struct {
	PodRequirements `json:",inline"`
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`
	//+kubebuilder:default:={port: 8091}
	Trillian TrillianService `json:"trillian,omitempty"`
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`
	//+kubebuilder:default:={enabled: true}
	RekorSearchUI RekorSearchUI `json:"rekorSearchUI,omitempty"`
	Signer RekorSigner `json:"signer,omitempty"`
	//+kubebuilder:default:={enabled: true, url: "file:///var/run/attestations?no_tmp_dir=true", maxSize: "100Ki"}
	Attestations RekorAttestations `json:"attestations,omitempty"`
	//+kubebuilder:default:={create: true}
	SearchIndex SearchIndex `json:"searchIndex,omitempty"`
	//+kubebuilder:default:={size: "5Gi", retain: true, accessModes: {ReadWriteOnce}}
	Pvc Pvc `json:"pvc,omitempty"`
	//+kubebuilder:default:={enabled: true, schedule: "0 0 * * *"}
	BackFillRedis BackFillRedis `json:"backFillRedis,omitempty"`
	// +listType=map
	// +listMapKey=treeID
	// +patchStrategy=merge
	// +patchMergeKey=treeID
	// +kubebuilder:default:={}
	Sharding []RekorLogRange `json:"sharding,omitempty"`
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
	//+optional
	Auth *Auth `json:"auth,omitempty"`
	//+kubebuilder:default:=10485760
	//+optional
	MaxRequestBodySize *int64 `json:"maxRequestBodySize,omitempty"`
}

type RekorAttestations struct {
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled once enabled.
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	// +kubebuilder:validation:XValidation:rule="(self.startsWith(\"file://\") || self.startsWith(\"s3://\") || self.startsWith(\"gs://\") || self.startsWith(\"azblob://\") || self.startsWith(\"mem://\"))",message="URL must use a supported protocol (file://, s3://, gs://, azblob://, mem://)."
	// +kubebuilder:validation:XValidation:rule="(!self.startsWith(\"file://\") || self.startsWith(\"file:///var/run/attestations\"))",message="If using 'file://' protocol, the URL must start with 'file:///var/run/attestations'."
	// +kubebuilder:default:="file:///var/run/attestations?no_tmp_dir=true"
	Url string `json:"url,omitempty"`
	// +kubebuilder:default:="100Ki"
	MaxSize *k8sresource.Quantity `json:"maxSize,omitempty"`
}

type RekorSigner struct {
	// +kubebuilder:validation:XValidation:rule="self == '' || self == 'secret' || self == 'memory' || self.matches('^awskms://.+$') || self.matches('^gcpkms://.+$') || self.matches('^azurekms://.+$') || self.matches('^hashivault://.+$')",message="KMS must be '', 'secret', 'memory', or a valid URI with a key path (e.g., awskms:///key-id)"
	// +kubebuilder:default:=secret
	KMS string `json:"kms,omitempty"`
	// +optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`
	// +optional
	KeyRef *SecretKeySelector `json:"keyRef,omitempty"`
}

type RekorSearchUI struct {
	PodRequirements `json:",inline"`
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	Host string `json:"host,omitempty"`
	RouteSelectorLabels map[string]string `json:"routeSelectorLabels,omitempty"`
}

// +kubebuilder:validation:XValidation:rule=(!(self.create == true) || !has(self.provider) || self.provider == ""),message=Provider can be specified only with external db (create=false)
// +kubebuilder:validation:XValidation:rule=(!(self.create == false) || self.provider != ""),message=Provider must be defined with external db (create=false)
// +kubebuilder:validation:XValidation:rule=(!(has(self.provider) && self.provider != "") || (self.url != "")),message=URL must be provided if provider is specified
type SearchIndex struct {
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Create *bool `json:"create"`
	//+optional
	TLS TLS `json:"tls,omitempty"`
	//+kubebuilder:validation:Enum={redis,mysql}
	Provider string `json:"provider,omitempty"`
	Url string `json:"url,omitempty"`
}

type BackFillRedis struct {
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	//+kubebuilder:default:="0 0 * * *"
	//+kubebuilder:validation:Pattern:="^(@(?i)(yearly|annually|monthly|weekly|daily|hourly)|((\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+\\s){4}(\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+)$"
	Schedule string `json:"schedule,omitempty"`
}

// +structType=atomic
type RekorLogRange struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	TreeID int64 `json:"treeID"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	TreeLength int64 `json:"treeLength"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9+/\n]+={0,2}\n*$`
	EncodedPublicKey string `json:"encodedPublicKey,omitempty"`
}

type SearchIndexStatus struct {
	TLS           TLS                `json:"tls,omitempty"`
	DbPasswordRef *SecretKeySelector `json:"dbPasswordRef,omitempty"`
}

type RekorStatus struct {
	PublicKeyRef     *SecretKeySelector    `json:"publicKeyRef,omitempty"`
	ServerConfigRef  *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Signer           RekorSigner           `json:"signer,omitempty"`
	SearchIndex      SearchIndexStatus     `json:"searchIndex,omitempty"`
	PvcName          string                `json:"pvcName,omitempty"`
	MonitorPvcName   string                `json:"monitorpvcName,omitempty"`
	Url              string                `json:"url,omitempty"`
	RekorSearchUIUrl string                `json:"rekorSearchUIUrl,omitempty"`
	// +kubebuilder:validation:Type=number
	TreeID *int64 `json:"treeID,omitempty"`
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

// Rekor is the Schema for the rekors API
// +kubebuilder:validation:XValidation:rule="(has(self.spec.attestations.enabled) && !self.spec.attestations.enabled) || !self.spec.attestations.url.startsWith('file://') || (!(self.spec.replicas > 1) || ('ReadWriteMany' in self.spec.pvc.accessModes))",message="When rich attestation storage is enabled, and it's URL starts with 'file://', then PVC accessModes must contain 'ReadWriteMany' for replicas greater than 1."
type Rekor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RekorSpec   `json:"spec,omitempty"`
	Status RekorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type RekorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rekor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rekor{}, &RekorList{})
}

func (i *Rekor) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Rekor) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *Rekor) GetTrustedCA() *LocalObjectReference {
	if i.Spec.TrustedCA != nil {
		return i.Spec.TrustedCA
	}
	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &LocalObjectReference{Name: v}
	}
	return nil
}
