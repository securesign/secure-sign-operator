package v1alpha1

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/securesign/operator/api/v1beta1"
)

// ConvertTo converts v1alpha1 Securesign (spoke) to v1beta1 Securesign (hub).
func (src *Securesign) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Securesign)

	dst.ObjectMeta = src.ObjectMeta

	// Fulcio (includes PKCS#11 round-trip via annotation)
	if err := convertFulcioSpecTo(&src.Spec.Fulcio, &dst.Spec.Fulcio, src.Annotations); err != nil {
		return fmt.Errorf("converting fulcio spec: %w", err)
	}

	// Rekor
	convertRekorSpecTo(&src.Spec.Rekor, &dst.Spec.Rekor)

	// Trillian
	convertTrillianSpecTo(&src.Spec.Trillian, &dst.Spec.Trillian)

	// TUF
	convertTufSpecTo(&src.Spec.Tuf, &dst.Spec.Tuf)

	// CTlog
	convertCTlogSpecTo(&src.Spec.Ctlog, &dst.Spec.Ctlog)

	// TSA
	convertTSASpecTo(src.Spec.TimestampAuthority, &dst.Spec)

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.RekorStatus = v1beta1.SecuresignRekorStatus{Url: src.Status.RekorStatus.Url}
	dst.Status.FulcioStatus = v1beta1.SecuresignFulcioStatus{Url: src.Status.FulcioStatus.Url}
	dst.Status.TufStatus = v1beta1.SecuresignTufStatus{Url: src.Status.TufStatus.Url}
	dst.Status.TSAStatus = v1beta1.SecuresignTSAStatus{Url: src.Status.TSAStatus.Url}

	// Clean up the round-trip annotation from dst if it was consumed
	if dst.Annotations != nil {
		delete(dst.Annotations, pkcs11ConfigAnnotation)
	}

	return nil
}

// ConvertFrom converts v1beta1 Securesign (hub) to v1alpha1 Securesign (spoke).
func (dst *Securesign) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.Securesign)

	dst.ObjectMeta = src.ObjectMeta

	// Fulcio (stash PKCS#11 config in annotation)
	if err := convertFulcioSpecFrom(&src.Spec.Fulcio, &dst.Spec.Fulcio, dst); err != nil {
		return fmt.Errorf("converting fulcio spec: %w", err)
	}

	// Rekor
	convertRekorSpecFrom(&src.Spec.Rekor, &dst.Spec.Rekor)

	// Trillian
	convertTrillianSpecFrom(&src.Spec.Trillian, &dst.Spec.Trillian)

	// TUF
	convertTufSpecFrom(&src.Spec.Tuf, &dst.Spec.Tuf)

	// CTlog
	convertCTlogSpecFrom(&src.Spec.Ctlog, &dst.Spec.Ctlog)

	// TSA
	convertTSASpecFrom(&src.Spec, dst)

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.RekorStatus = SecuresignRekorStatus{Url: src.Status.RekorStatus.Url}
	dst.Status.FulcioStatus = SecuresignFulcioStatus{Url: src.Status.FulcioStatus.Url}
	dst.Status.TufStatus = SecuresignTufStatus{Url: src.Status.TufStatus.Url}
	dst.Status.TSAStatus = SecuresignTSAStatus{Url: src.Status.TSAStatus.Url}

	return nil
}

// --- Fulcio sub-spec conversion helpers ---

func convertFulcioSpecTo(src *FulcioSpec, dst *v1beta1.FulcioSpec, annotations map[string]string) error {
	dst.ExternalAccess = convertExternalAccessTo(src.ExternalAccess)
	dst.Ctlog = convertCtlogServiceTo(src.Ctlog)
	dst.Config = convertFulcioConfigTo(src.Config)
	dst.Monitoring = convertMonitoringConfigTo(src.Monitoring)
	dst.TrustedCA = convertLocalObjectRefTo(src.TrustedCA)
	dst.Replicas = src.Replicas
	dst.Affinity = src.Affinity
	dst.Resources = src.Resources
	dst.Tolerations = src.Tolerations

	dst.Certificate.CAType = v1beta1.CATypeFile
	dst.Certificate.PrivateKeyRef = convertSecretKeySelectorTo(src.Certificate.PrivateKeyRef)
	dst.Certificate.PrivateKeyPasswordRef = convertSecretKeySelectorTo(src.Certificate.PrivateKeyPasswordRef)
	dst.Certificate.CARef = convertSecretKeySelectorTo(src.Certificate.CARef)
	dst.Certificate.CommonName = src.Certificate.CommonName
	dst.Certificate.OrganizationName = src.Certificate.OrganizationName
	dst.Certificate.OrganizationEmail = src.Certificate.OrganizationEmail

	if data, ok := annotations[pkcs11ConfigAnnotation]; ok {
		var pkcs11 v1beta1.PKCS11Config
		if err := json.Unmarshal([]byte(data), &pkcs11); err != nil {
			return fmt.Errorf("unmarshalling pkcs11 config annotation: %w", err)
		}
		dst.Certificate.CAType = v1beta1.CATypePKCS11
		dst.Certificate.PKCS11 = &pkcs11
	}

	return nil
}

func convertFulcioSpecFrom(src *v1beta1.FulcioSpec, dst *FulcioSpec, dstObj *Securesign) error {
	dst.ExternalAccess = convertExternalAccessFrom(src.ExternalAccess)
	dst.Ctlog = convertCtlogServiceFrom(src.Ctlog)
	dst.Config = convertFulcioConfigFrom(src.Config)
	dst.Monitoring = convertMonitoringConfigFrom(src.Monitoring)
	dst.TrustedCA = convertLocalObjectRefFrom(src.TrustedCA)
	dst.Replicas = src.Replicas
	dst.Affinity = src.Affinity
	dst.Resources = src.Resources
	dst.Tolerations = src.Tolerations

	dst.Certificate.PrivateKeyRef = convertSecretKeySelectorFrom(src.Certificate.PrivateKeyRef)
	dst.Certificate.PrivateKeyPasswordRef = convertSecretKeySelectorFrom(src.Certificate.PrivateKeyPasswordRef)
	dst.Certificate.CARef = convertSecretKeySelectorFrom(src.Certificate.CARef)
	dst.Certificate.CommonName = src.Certificate.CommonName
	dst.Certificate.OrganizationName = src.Certificate.OrganizationName
	dst.Certificate.OrganizationEmail = src.Certificate.OrganizationEmail

	if src.Certificate.CAType == v1beta1.CATypePKCS11 && src.Certificate.PKCS11 != nil {
		data, err := json.Marshal(src.Certificate.PKCS11)
		if err != nil {
			return fmt.Errorf("marshalling pkcs11 config: %w", err)
		}
		if dstObj.Annotations == nil {
			dstObj.Annotations = make(map[string]string)
		}
		dstObj.Annotations[pkcs11ConfigAnnotation] = string(data)
	}

	return nil
}

// --- Rekor sub-spec (pass-through, structurally identical) ---

func convertRekorSpecTo(src *RekorSpec, dst *v1beta1.RekorSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

func convertRekorSpecFrom(src *v1beta1.RekorSpec, dst *RekorSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

// --- Trillian sub-spec (pass-through) ---

func convertTrillianSpecTo(src *TrillianSpec, dst *v1beta1.TrillianSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

func convertTrillianSpecFrom(src *v1beta1.TrillianSpec, dst *TrillianSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

// --- TUF sub-spec (pass-through) ---

func convertTufSpecTo(src *TufSpec, dst *v1beta1.TufSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

func convertTufSpecFrom(src *v1beta1.TufSpec, dst *TufSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

// --- CTlog sub-spec (pass-through) ---

func convertCTlogSpecTo(src *CTlogSpec, dst *v1beta1.CTlogSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

func convertCTlogSpecFrom(src *v1beta1.CTlogSpec, dst *CTlogSpec) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
}

// --- TSA sub-spec (pass-through, optional) ---

func convertTSASpecTo(src *TimestampAuthoritySpec, dstSpec *v1beta1.SecuresignSpec) {
	if src == nil {
		dstSpec.TimestampAuthority = nil
		return
	}
	dst := &v1beta1.TimestampAuthoritySpec{}
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
	dstSpec.TimestampAuthority = dst
}

func convertTSASpecFrom(srcSpec *v1beta1.SecuresignSpec, dst *Securesign) {
	if srcSpec.TimestampAuthority == nil {
		dst.Spec.TimestampAuthority = nil
		return
	}
	dstTSA := &TimestampAuthoritySpec{}
	data, _ := json.Marshal(srcSpec.TimestampAuthority)
	_ = json.Unmarshal(data, dstTSA)
	dst.Spec.TimestampAuthority = dstTSA
}
