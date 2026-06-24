package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Rekor) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Rekor)

	// Standard auto-generated conversion (includes our manual RekorSpec conversion)
	if err := Convert_v1alpha1_Rekor_To_v1_Rekor(src, dst, nil); err != nil {
		return err
	}

	// Restore fields that don't exist in v1alpha1 from annotation (e.g., ImagePullSecrets)
	restored := &rhtasv1.Rekor{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets

	return nil
}

func (dst *Rekor) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Rekor)

	// Standard auto-generated conversion
	if err := Convert_v1_Rekor_To_v1alpha1_Rekor(src, dst, nil); err != nil {
		return err
	}

	// Restore the full v1alpha1 object from annotations (for roundtrip fidelity)
	// This will restore deprecated fields like spec.pvc that were preserved during ConvertTo
	restoredSpoke := &Rekor{}
	if hasAnnotation, err := utilconversion.UnmarshalData(src, restoredSpoke); err != nil {
		return err
	} else if hasAnnotation {
		// Restore deprecated spec.pvc field from annotation
		dst.Spec.Pvc = restoredSpoke.Spec.Pvc
	}

	// Marshal v1 (hub) data into annotations for fields that don't exist in v1alpha1 (spoke)
	return utilconversion.MarshalData(src, dst)
}

// Cross-type conversion: v1alpha1.RekorSigner (status) ↔ v1.RekorSignerStatus.
// v1 uses a dedicated RekorSignerStatus (without KMS) for status; v1alpha1 reuses RekorSigner.
func Convert_v1alpha1_RekorSigner_To_v1_RekorSignerStatus(in *RekorSigner, out *rhtasv1.RekorSignerStatus, s apiconversion.Scope) error {
	if in.PasswordRef != nil {
		out.PasswordRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PasswordRef, out.PasswordRef, s); err != nil {
			return err
		}
	}
	if in.KeyRef != nil {
		out.KeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.KeyRef, out.KeyRef, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1_RekorSignerStatus_To_v1alpha1_RekorSigner(in *rhtasv1.RekorSignerStatus, out *RekorSigner, s apiconversion.Scope) error {
	if in.PasswordRef != nil {
		out.PasswordRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.PasswordRef, out.PasswordRef, s); err != nil {
			return err
		}
	}
	if in.KeyRef != nil {
		out.KeyRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.KeyRef, out.KeyRef, s); err != nil {
			return err
		}
	}
	return nil
}
