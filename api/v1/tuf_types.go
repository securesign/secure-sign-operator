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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TufKeyRekor  = "rekor.pub"
	TufKeyCTFE   = "ctfe.pub"
	TufKeyFulcio = "fulcio_v1.crt.pem"
	TufKeyTSA    = "tsa.certchain.pem"
)

// TufSpec defines the desired state of Tuf
type TufSpec struct {
	PodRequirements      `json:",inline"`
	ServiceAccountConfig `json:",inline"`
	// Define whether you want to export service or not
	Ingress Ingress `json:"ingress,omitempty"`
	//+kubebuilder:validation:Minimum:=1
	//+kubebuilder:validation:Maximum:=65535
	Port int32 `json:"port,omitempty"`
	// Secret object reference that will hold you repository root keys. This parameter will be used only with operator-managed repository.
	RootKeySecretRef *LocalObjectReference `json:"rootKeySecretRef,omitempty"`
	// Pvc configuration of the persistent storage claim for deployment in the cluster.
	// You can use ReadWriteOnce accessMode if you don't have suitable storage provider but your deployment will not support HA mode
	Pvc Pvc `json:"pvc,omitempty"`
	// Ctlog service and trust material binding.
	//+optional
	// At most one entry is allowed today; this ceiling is deliberate and temporary,
	// pending future multi-instance support.
	//+kubebuilder:validation:MaxItems:=1
	// +listType=atomic
	//+kubebuilder:validation:XValidation:rule="self.all(x, !has(x.url) || size(x.url) == 0 || x.url.matches('^([a-zA-Z][a-zA-Z0-9+.-]*://[^/]+/.+|//[^/]*/.+)$'))",message="url must follow the pattern scheme://host[:port]/path or //[:port]/path"
	Ctlog []TrustRootBinding `json:"ctlog,omitempty"`
	// Fulcio service and trust material binding.
	//+optional
	// At most one entry is allowed today; this ceiling is deliberate and temporary,
	// pending future multi-instance support.
	//+kubebuilder:validation:MaxItems:=1
	// +listType=atomic
	//+kubebuilder:validation:XValidation:rule="self.all(x, !has(x.url) || size(x.url) == 0 || x.url.matches('^([a-zA-Z][a-zA-Z0-9+.-]*://[^/].*|//.+)$'))",message="url must follow the pattern scheme://host[:port][/path] or //[:port][/path]"
	Fulcio []TrustRootBindingWithOIDC `json:"fulcio,omitempty"`
	// Rekor service and trust material binding.
	//+optional
	// At most one entry is allowed today; this ceiling is deliberate and temporary,
	// pending future multi-instance support.
	//+kubebuilder:validation:MaxItems:=1
	// +listType=atomic
	//+kubebuilder:validation:XValidation:rule="self.all(x, !has(x.url) || size(x.url) == 0 || x.url.matches('^([a-zA-Z][a-zA-Z0-9+.-]*://[^/].*|//.+)$'))",message="url must follow the pattern scheme://host[:port][/path] or //[:port][/path]"
	Rekor []TrustRootBinding `json:"rekor,omitempty"`
	// TSA service and trust material binding. A nil value excludes TSA from the
	// trust root entirely; a non-nil value (even an empty list, meaning
	// autodiscover) includes it.
	//+optional
	// At most one entry is allowed today; this ceiling is deliberate and temporary,
	// pending future multi-instance support.
	//+kubebuilder:validation:MaxItems:=1
	// +listType=atomic
	//+kubebuilder:validation:XValidation:rule="self.all(x, !has(x.url) || size(x.url) == 0 || x.url.matches('^([a-zA-Z][a-zA-Z0-9+.-]*://[^/].*|//.+)$'))",message="url must follow the pattern scheme://host[:port][/path] or //[:port][/path]"
	Tsa *[]TrustRootBinding `json:"tsa,omitempty"`

	// ConfigMap with additional bundle of trusted CA
	// +optional
	TrustedCA     *LocalObjectReference `json:"trustedCA,omitempty"`
	PodExtensions `json:",inline"`
}

// TrustRootBinding identifies a component's service binding and, optionally, a
// reference to the Secret holding its trust material (public key for
// Rekor/CTlog, cert chain for Fulcio/TSA). If SecretRef is unset, the operator
// resolves material from the referenced or autodiscovered component CR's
// status.
// +kubebuilder:validation:XValidation:rule="!(has(self.ref) && has(self.secretRef))",message="ref and secretRef are mutually exclusive"
type TrustRootBinding struct {
	ServiceReference `json:",inline"`
	//+optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

// TrustRootBindingWithOIDC is a TrustRootBinding for Fulcio, which additionally
// carries the OIDC issuers accepted for code-signing certificate requests.
type TrustRootBindingWithOIDC struct {
	TrustRootBinding `json:",inline"`
	// OIDCIssuers is a list of OIDC issuer URLs to include in the Fulcio signing configuration.
	// Use for manual configuration; when specified, these values take precedence over auto-loaded OIDC configuration from the Fulcio service reference.
	//+optional
	//+listType=set
	//+kubebuilder:validation:items:Pattern=`^[a-zA-Z][a-zA-Z0-9+.-]*://.+$`
	OIDCIssuers []string `json:"oidcIssuers,omitempty"`
}

type TufKeyStatus struct {
	Name      string             `json:"name"`
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

// TufStatus defines the observed state of Tuf
type TufStatus struct {
	// +listType=map
	// +listMapKey=name
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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="The component status"
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,description="The component url"

// Tuf is the Schema for the tufs API
// +kubebuilder:validation:XValidation:rule="!has(self.spec.replicas) || !(self.spec.replicas > 1) || (has(self.spec.pvc.accessModes) && 'ReadWriteMany' in self.spec.pvc.accessModes)",message="For deployments with more than 1 replica, pvc.accessModes must include 'ReadWriteMany'."
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

func (i *Tuf) RemoveCondition(conditionType string) {
	meta.RemoveStatusCondition(&i.Status.Conditions, conditionType)
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
