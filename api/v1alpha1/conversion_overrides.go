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

// Manual conversion for Pvc to properly convert AccessModes slice
func Convert_v1alpha1_Pvc_To_v1_Pvc(in *Pvc, out *v1.Pvc, s apiconversion.Scope) error {
	out.Size = in.Size
	out.Retain = in.Retain
	out.Name = in.Name
	out.StorageClass = in.StorageClass

	// Convert AccessModes properly element by element
	if in.AccessModes != nil {
		out.AccessModes = make([]v1.PersistentVolumeAccessMode, len(in.AccessModes))
		for i, mode := range in.AccessModes {
			out.AccessModes[i] = v1.PersistentVolumeAccessMode(mode)
		}
	}
	return nil
}

func Convert_v1_Pvc_To_v1alpha1_Pvc(in *v1.Pvc, out *Pvc, s apiconversion.Scope) error {
	out.Size = in.Size
	out.Retain = in.Retain
	out.Name = in.Name
	out.StorageClass = in.StorageClass

	// Convert AccessModes properly element by element
	if in.AccessModes != nil {
		out.AccessModes = make([]PersistentVolumeAccessMode, len(in.AccessModes))
		for i, mode := range in.AccessModes {
			out.AccessModes[i] = PersistentVolumeAccessMode(mode)
		}
	}
	return nil
}

// Manual conversion for TufPvc to properly convert AccessModes slice
func Convert_v1alpha1_TufPvc_To_v1_TufPvc(in *TufPvc, out *v1.TufPvc, s apiconversion.Scope) error {
	out.Size = in.Size
	out.Retain = in.Retain
	out.Name = in.Name
	out.StorageClass = in.StorageClass

	// Convert AccessModes properly element by element
	if in.AccessModes != nil {
		out.AccessModes = make([]v1.PersistentVolumeAccessMode, len(in.AccessModes))
		for i, mode := range in.AccessModes {
			out.AccessModes[i] = v1.PersistentVolumeAccessMode(mode)
		}
	}
	return nil
}

func Convert_v1_TufPvc_To_v1alpha1_TufPvc(in *v1.TufPvc, out *TufPvc, s apiconversion.Scope) error {
	out.Size = in.Size
	out.Retain = in.Retain
	out.Name = in.Name
	out.StorageClass = in.StorageClass

	// Convert AccessModes properly element by element
	if in.AccessModes != nil {
		out.AccessModes = make([]PersistentVolumeAccessMode, len(in.AccessModes))
		for i, mode := range in.AccessModes {
			out.AccessModes[i] = PersistentVolumeAccessMode(mode)
		}
	}
	return nil
}
