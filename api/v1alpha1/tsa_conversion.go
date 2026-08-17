package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
)

// Convert_v1alpha1_File_To_v1_File is a manual conversion that drops the
// deprecated PasswordRef field which no longer exists in v1 File.
func Convert_v1alpha1_File_To_v1_File(in *File, out *rhtasv1.File, s apiconversion.Scope) error {
	if in.PrivateKeyRef != nil {
		out.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
			return err
		}
	} else {
		out.PrivateKeyRef = nil
	}
	return nil
}

// Convert_v1_File_To_v1alpha1_File is a manual conversion that handles
// the asymmetry: v1alpha1 File has PasswordRef but v1 File does not.
func Convert_v1_File_To_v1alpha1_File(in *rhtasv1.File, out *File, s apiconversion.Scope) error {
	if in.PrivateKeyRef != nil {
		out.PrivateKeyRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
			return err
		}
	} else {
		out.PrivateKeyRef = nil
	}
	return nil
}
