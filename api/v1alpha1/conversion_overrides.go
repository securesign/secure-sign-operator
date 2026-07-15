package v1alpha1

import (
	v1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func Convert_v1alpha1_RekorSpec_To_v1_RekorSpec(in *RekorSpec, out *v1.RekorSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_RekorSpec_To_v1_RekorSpec(in, out, s); err != nil {
		return err
	}
	return Convert_v1alpha1_Pvc_To_v1_Pvc(&in.Pvc, &out.Attestations.Pvc, s)
}

func Convert_v1_RekorSpec_To_v1alpha1_RekorSpec(in *v1.RekorSpec, out *RekorSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_RekorSpec_To_v1alpha1_RekorSpec(in, out, s); err != nil {
		return err
	}
	return Convert_v1_Pvc_To_v1alpha1_Pvc(&in.Attestations.Pvc, &out.Pvc, s)
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

func Convert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in *v1.RekorAttestations, out *RekorAttestations, s apiconversion.Scope) error {
	// Pvc is handled at the RekorSpec level conversion, not here.
	return autoConvert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in, out, s)
}

func Convert_v1alpha1_TlogMonitoring_To_v1_TlogMonitoring(in *TlogMonitoring, out *v1.TlogMonitoring, s apiconversion.Scope) error {
	if err := metav1.Convert_bool_To_Pointer_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	if in.Interval.Duration != 0 {
		interval := in.Interval
		out.Interval = &interval
	}
	return nil
}

func Convert_v1_TlogMonitoring_To_v1alpha1_TlogMonitoring(in *v1.TlogMonitoring, out *TlogMonitoring, s apiconversion.Scope) error {
	if err := metav1.Convert_Pointer_bool_To_bool(&in.Enabled, &out.Enabled, s); err != nil {
		return err
	}
	if err := metav1.Convert_Pointer_v1_Duration_To_v1_Duration(&in.Interval, &out.Interval, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha1_TufPvc_To_v1_Pvc(in *TufPvc, out *v1.Pvc, s apiconversion.Scope) error {
	pvc := Pvc(*in)
	return Convert_v1alpha1_Pvc_To_v1_Pvc(&pvc, out, s)
}

func Convert_v1_Pvc_To_v1alpha1_TufPvc(in *v1.Pvc, out *TufPvc, s apiconversion.Scope) error {
	var pvc Pvc
	if err := Convert_v1_Pvc_To_v1alpha1_Pvc(in, &pvc, s); err != nil {
		return err
	}
	*out = TufPvc(pvc)
	return nil
}
