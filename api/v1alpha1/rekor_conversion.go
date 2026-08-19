package v1alpha1

import (
	"unsafe"

	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"github.com/securesign/operator/internal/migration"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

var MigrationSearchUIData = migration.Key("v1alpha1", "rekorSearchUI")

func Convert_v1_RekorStatus_To_v1alpha1_RekorStatus(in *rhtasv1.RekorStatus, out *RekorStatus, s apiconversion.Scope) error {
	return autoConvert_v1_RekorStatus_To_v1alpha1_RekorStatus(in, out, s)
}

func Convert_v1alpha1_RekorStatus_To_v1_RekorStatus(in *RekorStatus, out *rhtasv1.RekorStatus, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_RekorStatus_To_v1_RekorStatus(in, out, s)
}

func Convert_v1alpha1_RekorSpec_To_v1_RekorSpec(in *RekorSpec, out *rhtasv1.RekorSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_RekorSpec_To_v1_RekorSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s); err != nil {
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
	return Convert_v1_Pvc_To_v1alpha1_Pvc(&in.Attestations.Pvc, &out.Pvc, s)
}

func Convert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in *rhtasv1.RekorAttestations, out *RekorAttestations, s apiconversion.Scope) error {
	return autoConvert_v1_RekorAttestations_To_v1alpha1_RekorAttestations(in, out, s)
}

func (src *Rekor) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Rekor)
	if err := Convert_v1alpha1_Rekor_To_v1_Rekor(src, dst, nil); err != nil {
		return err
	}

	if err := migration.Set(dst, MigrationSearchUIData, src.Spec.RekorSearchUI); err != nil {
		return err
	}

	restored := &rhtasv1.Rekor{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Status.PublicKey = restored.Status.PublicKey
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	if dst.Spec.Trillian.URL == "" {
		dst.Spec.Trillian.Ref = restored.Spec.Trillian.Ref
	}
	if dst.Spec.Monitoring.Tuf.URL == "" {
		dst.Spec.Monitoring.Tuf.Ref = restored.Spec.Monitoring.Tuf.Ref
	}
	dst.Spec.PodExtensions = restored.Spec.PodExtensions

	return nil
}

func (dst *Rekor) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Rekor)
	if err := Convert_v1_Rekor_To_v1alpha1_Rekor(src, dst, nil); err != nil {
		return err
	}

	var searchUI RekorSearchUI
	if ok, err := migration.Pop(src, MigrationSearchUIData, &searchUI); err != nil {
		return err
	} else if ok {
		dst.Spec.RekorSearchUI = searchUI
	}

	return utilconversion.MarshalData(src, dst)
}

// v1alpha1 flat KMS string → v1 struct-based signer.
func Convert_v1alpha1_RekorSigner_To_v1_RekorSigner(in *RekorSigner, out *rhtasv1.RekorSigner, s apiconversion.Scope) error {
	switch in.KMS {
	case rhtasv1.RekorSignerTypeSecret:
		out.Type = rhtasv1.RekorSignerTypeSecret
	case rhtasv1.RekorSignerTypeMemory:
		out.Type = rhtasv1.RekorSignerTypeMemory
	case "":
		out.Type = ""
	default:
		out.Type = rhtasv1.RekorSignerTypeKMS
		out.Kms = &rhtasv1.KMS{KeyResource: in.KMS}
	}
	out.KeyRef = (*rhtasv1.SecretKeySelector)(unsafe.Pointer(in.KeyRef))
	return nil
}

// v1 struct-based signer → v1alpha1 flat KMS string.
func Convert_v1_RekorSigner_To_v1alpha1_RekorSigner(in *rhtasv1.RekorSigner, out *RekorSigner, s apiconversion.Scope) error {
	switch in.Type {
	case rhtasv1.RekorSignerTypeKMS:
		if in.Kms != nil {
			out.KMS = in.Kms.KeyResource
		}
	case rhtasv1.RekorSignerTypeMemory:
		out.KMS = rhtasv1.RekorSignerTypeMemory
	case rhtasv1.RekorSignerTypeSecret:
		out.KMS = rhtasv1.RekorSignerTypeSecret
	default:
		out.KMS = in.Type
	}
	out.KeyRef = (*SecretKeySelector)(unsafe.Pointer(in.KeyRef))
	return nil
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
