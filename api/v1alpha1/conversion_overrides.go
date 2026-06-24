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

func Convert_v1_TimestampAuthoritySpec_To_v1alpha1_TimestampAuthoritySpec(in *v1.TimestampAuthoritySpec, out *TimestampAuthoritySpec, s apiconversion.Scope) error {
	return autoConvert_v1_TimestampAuthoritySpec_To_v1alpha1_TimestampAuthoritySpec(in, out, s)
}

func Convert_v1_TrillianSpec_To_v1alpha1_TrillianSpec(in *v1.TrillianSpec, out *TrillianSpec, s apiconversion.Scope) error {
	return autoConvert_v1_TrillianSpec_To_v1alpha1_TrillianSpec(in, out, s)
}

func Convert_v1_TufSpec_To_v1alpha1_TufSpec(in *v1.TufSpec, out *TufSpec, s apiconversion.Scope) error {
	return autoConvert_v1_TufSpec_To_v1alpha1_TufSpec(in, out, s)
}

func Convert_v1_RekorSpec_To_v1alpha1_RekorSpec(in *v1.RekorSpec, out *RekorSpec, s apiconversion.Scope) error {
	// First call autoConvert to handle all standard fields
	if err := autoConvert_v1_RekorSpec_To_v1alpha1_RekorSpec(in, out, s); err != nil {
		return err
	}

	// For backward compatibility with v1alpha1 clients that still use the deprecated spec.pvc field,
	// populate it from v1's spec.attestations.pvc (the authoritative location in v1).
	// The autoConvert copies in.Pvc (which is deprecated/empty in v1) to out.Pvc,
	// but we want out.Pvc to mirror in.Attestations.Pvc for old v1alpha1 clients.
	if err := Convert_v1_Pvc_To_v1alpha1_Pvc(&in.Attestations.Pvc, &out.Pvc, s); err != nil {
		return err
	}

	return nil
}

// Manual conversion for RekorSpec to properly convert PVC AccessModes slice and handle PVC migration
func Convert_v1alpha1_RekorSpec_To_v1_RekorSpec(in *RekorSpec, out *v1.RekorSpec, s apiconversion.Scope) error {
	// Check if we need to migrate old spec.pvc to spec.attestations.pvc BEFORE autoConvert runs
	// (autoConvert will apply defaults to in.Attestations.Pvc, making it appear non-empty)
	isPvcEmpty := in.Pvc.Size == nil && in.Pvc.Retain == nil && in.Pvc.Name == "" && in.Pvc.StorageClass == "" && len(in.Pvc.AccessModes) == 0
	isAttestationsPvcEmpty := in.Attestations.Pvc.Size == nil && in.Attestations.Pvc.Retain == nil && in.Attestations.Pvc.Name == "" && in.Attestations.Pvc.StorageClass == "" && len(in.Attestations.Pvc.AccessModes) == 0
	shouldMigrate := !isPvcEmpty && isAttestationsPvcEmpty

	// Call autoConvert to handle all standard fields
	if err := autoConvert_v1alpha1_RekorSpec_To_v1_RekorSpec(in, out, s); err != nil {
		return err
	}

	// Then do custom PVC AccessModes conversion for the deprecated Pvc field
	if in.Pvc.AccessModes != nil {
		out.Pvc.AccessModes = make([]v1.PersistentVolumeAccessMode, len(in.Pvc.AccessModes))
		for i, mode := range in.Pvc.AccessModes {
			out.Pvc.AccessModes[i] = v1.PersistentVolumeAccessMode(mode)
		}
	}

	// Migrate old spec.pvc to spec.attestations.pvc if needed
	// This overwrites any defaults that autoConvert applied
	if shouldMigrate {
		if err := Convert_v1alpha1_Pvc_To_v1_Pvc(&in.Pvc, &out.Attestations.Pvc, s); err != nil {
			return err
		}
	}

	// Clear the deprecated out.Pvc field in v1 after migration
	// The authoritative location in v1 is spec.attestations.pvc
	out.Pvc = v1.Pvc{}

	return nil
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
