package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RekorSpec defines the desired state of Rekor
type RekorSpec struct {
	PodRequirements `json:",inline"`
	// ID of Merkle tree in Trillian backend
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`
	// Trillian service configuration
	//+kubebuilder:default:={port: 8091}
	Trillian TrillianService `json:"trillian,omitempty"`
	// Define whether you want to export service or not
	ExternalAccess ExternalAccess `json:"externalAccess,omitempty"`
	//Enable Service monitors for rekor
	Monitoring MonitoringWithTLogConfig `json:"monitoring,omitempty"`
	// Rekor Search UI
	//+kubebuilder:default:={enabled: true}
	RekorSearchUI RekorSearchUI `json:"rekorSearchUI,omitempty"`
	// Signer configuration
	Signer RekorSigner `json:"signer,omitempty"`
	// Attestations configuration
	//+kubebuilder:default:={enabled: true, url: "file:///var/run/attestations?no_tmp_dir=true", maxSize: "100Ki"}
	Attestations RekorAttestations `json:"attestations,omitempty"`
	// Define your search index database connection
	//+kubebuilder:default:={create: true}
	SearchIndex SearchIndex `json:"searchIndex,omitempty"`
	// PVC configuration
	//+kubebuilder:default:={size: "5Gi", retain: true, accessModes: {ReadWriteOnce}}
	Pvc Pvc `json:"pvc,omitempty"`
	// BackFillRedis CronJob Configuration
	//+kubebuilder:default:={enabled: true, schedule: "0 0 * * *"}
	BackFillRedis BackFillRedis `json:"backFillRedis,omitempty"`
	// Inactive shards
	// +listType=map
	// +listMapKey=treeID
	// +patchStrategy=merge
	// +patchMergeKey=treeID
	// +kubebuilder:default:={}
	Sharding []RekorLogRange `json:"sharding,omitempty"`
	// ConfigMap with additional bundle of trusted CA
	//+optional
	TrustedCA *LocalObjectReference `json:"trustedCA,omitempty"`
	//Configuration for authentication for key management services
	//+optional
	Auth *Auth `json:"auth,omitempty"`

	// MaxRequestBodySize sets the maximum size in bytes for HTTP request body. Passed as --max_request_body_size.
	//+kubebuilder:default:=10485760
	//+optional
	MaxRequestBodySize *int64 `json:"maxRequestBodySize,omitempty"`

	ServiceAccountRequirements `json:",inline"`
}

// RekorAttestations defines the configuration for storing attestations.
type RekorAttestations struct {
	// Enabled specifies whether the rich attestation storage feature should be enabled.
	// When set to true, the system will store detailed attestations.
	// This feature cannot be disabled once enabled to maintain data integrity.
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled once enabled.
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`

	/// Url specifies the storage location for attestations, supporting go-cloud blob URLs.
	// The "file:///var/run/attestations" path is specifically for local storage
	// that relies on a mounted Persistent Volume Claim (PVC) for data persistence.
	// Other valid protocols include s3://, gs://, azblob://, and mem://.
	//
	// Examples of valid URLs:
	// - Amazon S3: "s3://my-bucket?region=us-west-1"
	// - S3-Compatible Storage: "s3://my-bucket?endpoint=my.minio.local:8080&s3ForcePathStyle=true"
	// - Google Cloud Storage: "gs://my-bucket"
	// - Azure Blob Storage: "azblob://my-container"
	// - In-memory (for testing/development): "mem://"
	// - Local file system: "file:///var/run/attestations?no_tmp_dir=true"
	//
	// +kubebuilder:validation:XValidation:rule="(self.startsWith(\"file://\") || self.startsWith(\"s3://\") || self.startsWith(\"gs://\") || self.startsWith(\"azblob://\") || self.startsWith(\"mem://\"))",message="URL must use a supported protocol (file://, s3://, gs://, azblob://, mem://)."
	// +kubebuilder:validation:XValidation:rule="(!self.startsWith(\"file://\") || self.startsWith(\"file:///var/run/attestations\"))",message="If using 'file://' protocol, the URL must start with 'file:///var/run/attestations'."
	// +kubebuilder:default:="file:///var/run/attestations?no_tmp_dir=true"
	Url string `json:"url,omitempty"`

	// MaxSize defines the maximum allowed size for an individual attestation.
	// This helps prevent excessively large attestations from being stored.
	// +kubebuilder:default:="100Ki"
	MaxSize *k8sresource.Quantity `json:"maxSize,omitempty"`
}

type RekorSigner struct {

	// KMS Signer provider. Specifies the key management system (KMS) used for signing operations.
	//
	// Valid values:
	// - "secret" (default): The signer key is stored in a Kubernetes Secret.
	// - "memory": Ephemeral signer key stored in memory. Recommended for development use only.
	// - KMS URI: A URI to a cloud-based KMS, following the Go Cloud Development Kit (Go Cloud) URI format. Supported URIs include:
	//   - awskms://keyname
	//   - azurekms://keyname
	//   - gcpkms://keyname
	//   - hashivault://keyname
	// +kubebuilder:validation:XValidation:rule="self == '' || self == 'secret' || self == 'memory' || self.matches('^awskms://.+$') || self.matches('^gcpkms://.+$') || self.matches('^azurekms://.+$') || self.matches('^hashivault://.+$')",message="KMS must be '', 'secret', 'memory', or a valid URI with a key path (e.g., awskms:///key-id)"
	// +kubebuilder:default:=secret
	KMS string `json:"kms,omitempty"`

	// Password to decrypt the signer private key.
	//
	// Optional field. This should be set only if the private key referenced by `keyRef` is encrypted with a password.
	// If KMS is set to a value other than "secret", this field is ignored.
	// +optional
	PasswordRef *SecretKeySelector `json:"passwordRef,omitempty"`

	// Reference to the signer private key.
	//
	// Optional field. When KMS is set to "secret", this field can be left empty, in which case the operator will automatically generate a signer key.
	// +optional
	KeyRef *SecretKeySelector `json:"keyRef,omitempty"`
}

type RekorSearchUI struct {
	PodRequirements `json:",inline"`
	// If set to true, the Operator will deploy a Rekor Search UI
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
	// Set Route Selector Labels labels for ingress sharding.
	RouteSelectorLabels map[string]string `json:"routeSelectorLabels,omitempty"`
}

// SearchIndex define search index connection
// +kubebuilder:validation:XValidation:rule=(!(self.create == true) || !has(self.provider) || self.provider == ""),message=Provider can be specified only with external db (create=false)
// +kubebuilder:validation:XValidation:rule=(!(self.create == false) || self.provider != ""),message=Provider must be defined with external db (create=false)
// +kubebuilder:validation:XValidation:rule=(!(has(self.provider) && self.provider != "") || (self.url != "")),message=URL must be provided if provider is specified
type SearchIndex struct {
	// Create Database if a database. If create=true provider and url fields are not taken into account, otherwise url field must be specified.
	//+kubebuilder:default:=true
	//+kubebuilder:validation:XValidation:rule=(self == oldSelf),message=Field is immutable
	Create *bool `json:"create"`
	// Configuration for enabling TLS (Transport Layer Security) encryption for manged database.
	//+optional
	TLS TLS `json:"tls,omitempty"`
	// DB provider. Supported are redis and mysql.
	//+kubebuilder:validation:Enum={redis,mysql}
	Provider string `json:"provider,omitempty"`
	// DB connection URL.
	Url string `json:"url,omitempty"`
}

type BackFillRedis struct {
	//Enable the BackFillRedis CronJob
	//+kubebuilder:validation:XValidation:rule=(self || !oldSelf),message=Feature cannot be disabled
	//+kubebuilder:default:=true
	Enabled *bool `json:"enabled"`
	//Schedule for the BackFillRedis CronJob
	//+kubebuilder:default:="0 0 * * *"
	//+kubebuilder:validation:Pattern:="^(@(?i)(yearly|annually|monthly|weekly|daily|hourly)|((\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+\\s){4}(\\*(\\/[1-9][0-9]*)?|[0-9,-]+)+)$"
	Schedule string `json:"schedule,omitempty"`
}

// RekorLogRange defines the range and details of a log shard
// +structType=atomic
type RekorLogRange struct {
	// ID of Merkle tree in Trillian backend
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	TreeID int64 `json:"treeID"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// Length of the tree
	TreeLength int64 `json:"treeLength"`
	// The public key for the log shard, encoded in Base64 format
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9+/\n]+={0,2}\n*$`
	EncodedPublicKey string `json:"encodedPublicKey,omitempty"`
}

type SearchIndexStatus struct {
	TLS           TLS                `json:"tls,omitempty"`
	DbPasswordRef *SecretKeySelector `json:"dbPasswordRef,omitempty"`
}

// RekorStatus defines the observed state of Rekor
type RekorStatus struct {
	// Reference to secret with Rekor's signer public key.
	// Public key is automatically generated from signer private key.
	PublicKeyRef     *SecretKeySelector    `json:"publicKeyRef,omitempty"`
	ServerConfigRef  *LocalObjectReference `json:"serverConfigRef,omitempty"`
	Signer           RekorSigner           `json:"signer,omitempty"`
	SearchIndex      SearchIndexStatus     `json:"searchIndex,omitempty"`
	PvcName          string                `json:"pvcName,omitempty"`
	MonitorPvcName   string                `json:"monitorpvcName,omitempty"`
	Url              string                `json:"url,omitempty"`
	RekorSearchUIUrl string                `json:"rekorSearchUIUrl,omitempty"`
	// The ID of a Trillian tree that stores the log data.
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

// RekorList contains a list of Rekor
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
		return &LocalObjectReference{
			Name: v,
		}
	}

	return nil
}
