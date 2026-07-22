package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1_RekorStatus_To_v1alpha1_RekorStatus(in *rhtasv1.RekorStatus, out *RekorStatus, s apiconversion.Scope) error {
	return autoConvert_v1_RekorStatus_To_v1alpha1_RekorStatus(in, out, s)
}

func Convert_v1alpha1_RekorSpec_To_v1_RekorSpec(in *RekorSpec, out *rhtasv1.RekorSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_RekorSpec_To_v1_RekorSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_RekorSearchUI_To_v1_RekorSearchUI(&in.RekorSearchUI, &out.RekorSearchUI, s); err != nil {
		return err
	}
	return Convert_v1alpha1_Pvc_To_v1_Pvc(&in.Pvc, &out.Attestations.Pvc, s)
}

func Convert_v1_RekorSpec_To_v1alpha1_RekorSpec(in *rhtasv1.RekorSpec, out *RekorSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_RekorSpec_To_v1alpha1_RekorSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1_Ingress_To_v1alpha1_ExternalAccess(&in.Ingress, &out.ExternalAccess, s); err != nil {
		return err
	}
	if err := Convert_v1_RekorSearchUI_To_v1alpha1_RekorSearchUI(&in.RekorSearchUI, &out.RekorSearchUI, s); err != nil {
		return err
	}
	return Convert_v1_Pvc_To_v1alpha1_Pvc(&in.Attestations.Pvc, &out.Pvc, s)
}

// RekorSearchUI had its RouteSelectorLabels field renamed to Labels in v1.

func Convert_v1alpha1_RekorSearchUI_To_v1_RekorSearchUI(in *RekorSearchUI, out *rhtasv1.RekorSearchUI, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_RekorSearchUI_To_v1_RekorSearchUI(in, out, s); err != nil {
		return err
	}
	out.Labels = in.RouteSelectorLabels
	return nil
}

func Convert_v1_RekorSearchUI_To_v1alpha1_RekorSearchUI(in *rhtasv1.RekorSearchUI, out *RekorSearchUI, s apiconversion.Scope) error {
	if err := autoConvert_v1_RekorSearchUI_To_v1alpha1_RekorSearchUI(in, out, s); err != nil {
		return err
	}
	out.RouteSelectorLabels = in.Labels
	return nil
}

func Convert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in *rhtasv1.RekorAttestations, out *RekorAttestations, s apiconversion.Scope) error {
	// Pvc is handled at the RekorSpec level conversion, not here.
	return autoConvert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in, out, s)
}

func (src *Rekor) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Rekor)
	if err := Convert_v1alpha1_Rekor_To_v1_Rekor(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Rekor{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Status.PublicKey = restored.Status.PublicKey
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	return nil
}

func (dst *Rekor) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Rekor)
	if err := Convert_v1_Rekor_To_v1alpha1_Rekor(src, dst, nil); err != nil {
		return err
	}
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
