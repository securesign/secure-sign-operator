package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1alpha1_TrillianDB_To_v1_TrillianDBStatus(in *TrillianDB, out *rhtasv1.TrillianDBStatus, s apiconversion.Scope) error {
	out.PvcName = in.Pvc.Name
	if in.DatabaseSecretRef != nil {
		out.DatabaseSecretRef = &rhtasv1.LocalObjectReference{}
		if err := Convert_v1alpha1_LocalObjectReference_To_v1_LocalObjectReference(in.DatabaseSecretRef, out.DatabaseSecretRef, s); err != nil {
			return err
		}
	}
	return Convert_v1alpha1_TLS_To_v1_TLS(&in.TLS, &out.TLS, s)
}

func Convert_v1_TrillianDBStatus_To_v1alpha1_TrillianDB(in *rhtasv1.TrillianDBStatus, out *TrillianDB, s apiconversion.Scope) error {
	out.Pvc.Name = in.PvcName
	if in.DatabaseSecretRef != nil {
		out.DatabaseSecretRef = &LocalObjectReference{}
		if err := Convert_v1_LocalObjectReference_To_v1alpha1_LocalObjectReference(in.DatabaseSecretRef, out.DatabaseSecretRef, s); err != nil {
			return err
		}
	}
	return Convert_v1_TLS_To_v1alpha1_TLS(&in.TLS, &out.TLS, s)
}

func Convert_v1alpha1_TrillianLogServer_To_v1_TrillianServiceStatus(in *TrillianLogServer, out *rhtasv1.TrillianServiceStatus, s apiconversion.Scope) error {
	return Convert_v1alpha1_TLS_To_v1_TLS(&in.TLS, &out.TLS, s)
}

func Convert_v1_TrillianServiceStatus_To_v1alpha1_TrillianLogServer(in *rhtasv1.TrillianServiceStatus, out *TrillianLogServer, s apiconversion.Scope) error {
	return Convert_v1_TLS_To_v1alpha1_TLS(&in.TLS, &out.TLS, s)
}

func Convert_v1alpha1_TrillianLogSigner_To_v1_TrillianServiceStatus(in *TrillianLogSigner, out *rhtasv1.TrillianServiceStatus, s apiconversion.Scope) error {
	return Convert_v1alpha1_TLS_To_v1_TLS(&in.TLS, &out.TLS, s)
}

func Convert_v1_TrillianServiceStatus_To_v1alpha1_TrillianLogSigner(in *rhtasv1.TrillianServiceStatus, out *TrillianLogSigner, s apiconversion.Scope) error {
	return Convert_v1_TLS_To_v1alpha1_TLS(&in.TLS, &out.TLS, s)
}

func (src *Trillian) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Trillian)
	if err := Convert_v1alpha1_Trillian_To_v1_Trillian(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Trillian{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	return nil
}

func (dst *Trillian) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Trillian)
	if err := Convert_v1_Trillian_To_v1alpha1_Trillian(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
