package v1alpha1

import (
	v1 "github.com/securesign/operator/api/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
)

// Manual conversion functions for Spec types where v1 has fields
// that don't exist in v1alpha1 (e.g. ServiceAccountConfig).
// These fields are preserved via MarshalData/UnmarshalData annotation.

func Convert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in *v1.CTlogSpec, out *CTlogSpec, s apiconversion.Scope) error {
	return autoConvert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in, out, s)
}

func Convert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in *v1.FulcioSpec, out *FulcioSpec, s apiconversion.Scope) error {
	return autoConvert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in, out, s)
}

func Convert_v1_RekorSpec_To_v1alpha1_RekorSpec(in *v1.RekorSpec, out *RekorSpec, s apiconversion.Scope) error {
	return autoConvert_v1_RekorSpec_To_v1alpha1_RekorSpec(in, out, s)
}

func Convert_v1_TimestampAuthoritySpec_To_v1alpha1_TimestampAuthoritySpec(in *v1.TimestampAuthoritySpec, out *TimestampAuthoritySpec, s apiconversion.Scope) error {
	return autoConvert_v1_TimestampAuthoritySpec_To_v1alpha1_TimestampAuthoritySpec(in, out, s)
}

func Convert_v1_TrillianSpec_To_v1alpha1_TrillianSpec(in *v1.TrillianSpec, out *TrillianSpec, s apiconversion.Scope) error {
	return autoConvert_v1_TrillianSpec_To_v1alpha1_TrillianSpec(in, out, s)
}

func Convert_v1_TufSpec_To_v1alpha1_TufSpec(in *v1.TufSpec, out *TufSpec, s apiconversion.Scope) error {
	return autoConvert_v1_TufSpec_To_v1alpha1_TufSpec(in, out, s)
}

// Manual conversion functions for Status types where v1 has fields
// that don't exist in v1alpha1 (e.g. PublicKey, CertificateChain).
// These fields are preserved via MarshalData/UnmarshalData annotation.

func Convert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in *v1.CTlogStatus, out *CTlogStatus, s apiconversion.Scope) error {
	return autoConvert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in, out, s)
}

func Convert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in *v1.FulcioStatus, out *FulcioStatus, s apiconversion.Scope) error {
	return autoConvert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in, out, s)
}

func Convert_v1_RekorStatus_To_v1alpha1_RekorStatus(in *v1.RekorStatus, out *RekorStatus, s apiconversion.Scope) error {
	return autoConvert_v1_RekorStatus_To_v1alpha1_RekorStatus(in, out, s)
}
