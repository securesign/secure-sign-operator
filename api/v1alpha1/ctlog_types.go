package v1alpha1

import (
	"encoding/json"
	"github.com/securesign/operator/api/common"
	v1 "github.com/securesign/operator/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CTlogSpec defines the desired state of CTlog component
// +kubebuilder:validation:XValidation:rule=(!has(self.publicKeyRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
// +kubebuilder:validation:XValidation:rule=(!has(self.privateKeyPasswordRef) || has(self.privateKeyRef)),message=privateKeyRef cannot be empty
type CTlogSpec struct {
	common.PodRequirements `json:",inline"`
	// The ID of a Trillian tree that stores the log data.
	// If it is unset, the operator will create new Merkle tree in the Trillian backend
	//+optional
	TreeID *int64 `json:"treeID,omitempty"`

	// The private key used for signing STHs etc.
	//+optional
	PrivateKeyRef *common.SecretKeySelector `json:"privateKeyRef,omitempty"`

	// Password to decrypt private key
	//+optional
	PrivateKeyPasswordRef *common.SecretKeySelector `json:"privateKeyPasswordRef,omitempty"`

	// The public key matching the private key (if both are present). It is
	// used only by mirror logs for verifying the source log's signatures, but can
	// be specified for regular logs as well for the convenience of test tools.
	//+optional
	PublicKeyRef *common.SecretKeySelector `json:"publicKeyRef,omitempty"`

	// List of secrets containing root certificates that are acceptable to the log.
	// The certs are served through get-roots endpoint. Optional in mirrors.
	//+optional
	RootCertificates []common.SecretKeySelector `json:"rootCertificates,omitempty"`

	//Enable Service monitors for ctlog
	Monitoring common.MonitoringWithTLogConfig `json:"monitoring,omitempty"`

	// Trillian service configuration
	//+kubebuilder:default:={port: 8091}
	Trillian common.TrillianService `json:"trillian,omitempty"`

	// Secret holding Certificate Transparency server config in text proto format
	// If it is set then any setting of treeID, privateKeyRef, privateKeyPasswordRef,
	// publicKeyRef, rootCertificates and trillian will be overridden.
	//+optional
	ServerConfigRef *common.LocalObjectReference `json:"serverConfigRef,omitempty"`

	// Configuration for enabling common.TLS (Transport Layer Security) encryption for manged service.
	//+optional
	common.TLS `json:"tls,omitempty"`

	// Max certificate chain size in bytes. Passed as --max_cert_chain_size.
	//+kubebuilder:default:=153600
	//+optional
	MaxCertChainSize *int64 `json:"maxCertChainSize,omitempty"`
}

// CTlogStatus defines the observed state of CTlog component
type CTlogStatus struct {
	ServerConfigRef       *common.LocalObjectReference `json:"serverConfigRef,omitempty"`
	PrivateKeyRef         *common.SecretKeySelector    `json:"privateKeyRef,omitempty"`
	PrivateKeyPasswordRef *common.SecretKeySelector    `json:"privateKeyPasswordRef,omitempty"`
	PublicKeyRef          *common.SecretKeySelector    `json:"publicKeyRef,omitempty"`
	RootCertificates      []common.SecretKeySelector   `json:"rootCertificates,omitempty"`
	// The ID of a Trillian tree that stores the log data.
	// +kubebuilder:validation:Type=number
	TreeID *int64 `json:"treeID,omitempty"`
	// Configuration for enabling common.TLS (Transport Layer Security) encryption for manged service.
	//+optional
	common.TLS `json:"tls,omitempty"`
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

// CTlog is the Schema for the ctlogs API
type CTlog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CTlogSpec   `json:"spec,omitempty"`
	Status CTlogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CTlogList contains a list of CTlog
type CTlogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CTlog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CTlog{}, &CTlogList{})
}

func (i *CTlog) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *CTlog) SetCondition(newCondition metav1.Condition) {
	meta.SetStatusCondition(&i.Status.Conditions, newCondition)
}

func (i *CTlog) GetTrustedCA() *common.LocalObjectReference {
	if v, ok := i.GetAnnotations()["rhtas.redhat.com/trusted-ca"]; ok {
		return &common.LocalObjectReference{
			Name: v,
		}
	}

	return nil
}

// ConvertTo converts this v1alpha1 CTlog to the Hub version (v1)
func (src *CTlog) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1.CTlog)

	// Copy metadata directly
	dst.ObjectMeta = src.ObjectMeta

	// Convert Spec via JSON (safe since schemas are identical)
	bytes, err := json.Marshal(src.Spec)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &dst.Spec); err != nil {
		return err
	}

	// Convert Status via JSON
	bytes, err = json.Marshal(src.Status)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, &dst.Status)
}

// ConvertFrom converts from the Hub version (v1) to this v1alpha1 CTlog
func (dst *CTlog) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1.CTlog)

	// Copy metadata directly
	dst.ObjectMeta = src.ObjectMeta

	// Convert Spec via JSON
	bytes, err := json.Marshal(src.Spec)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &dst.Spec); err != nil {
		return err
	}

	// Convert Status via JSON
	bytes, err = json.Marshal(src.Status)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, &dst.Status)
}

// SetupWebhookWithManager sets up the conversion webhook with the Manager
func (r *CTlog) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).Complete()
}
