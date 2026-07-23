package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Tuf) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Tuf)
	if err := Convert_v1alpha1_Tuf_To_v1_Tuf(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Tuf{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Spec.TrustedCA = restored.Spec.TrustedCA
	if dst.Spec.Rekor.URL == "" {
		dst.Spec.Rekor.Ref = restored.Spec.Rekor.Ref
	}
	if dst.Spec.Fulcio.URL == "" {
		dst.Spec.Fulcio.Ref = restored.Spec.Fulcio.Ref
	}
	dst.Spec.Fulcio.OIDCIssuers = restored.Spec.Fulcio.OIDCIssuers
	if dst.Spec.Ctlog.URL == "" {
		dst.Spec.Ctlog.Ref = restored.Spec.Ctlog.Ref
	}
	if dst.Spec.Tsa.URL == "" {
		dst.Spec.Tsa.Ref = restored.Spec.Tsa.Ref
	}
	return nil
}

func (dst *Tuf) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Tuf)
	if err := Convert_v1_Tuf_To_v1alpha1_Tuf(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}

func Convert_v1alpha1_TufSpec_To_v1_TufSpec(in *TufSpec, out *rhtasv1.TufSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_TufSpec_To_v1_TufSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_CtlogService_To_v1_ServiceReference(&in.Ctlog, &out.Ctlog, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_FulcioService_To_v1_ServiceRefWithOIDC(&in.Fulcio, &out.Fulcio, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_RekorService_To_v1_ServiceReference(&in.Rekor, &out.Rekor, s); err != nil {
		return err
	}
	if err := Convert_v1alpha1_TsaService_To_v1_ServiceReference(&in.Tsa, &out.Tsa, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1_TufSpec_To_v1alpha1_TufSpec(in *rhtasv1.TufSpec, out *TufSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_TufSpec_To_v1alpha1_TufSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1_Ingress_To_v1alpha1_ExternalAccess(&in.Ingress, &out.ExternalAccess, s); err != nil {
		return err
	}
	if err := Convert_v1_ServiceReference_To_v1alpha1_CtlogService(&in.Ctlog, &out.Ctlog, s); err != nil {
		return err
	}
	if err := Convert_v1_ServiceRefWithOIDC_To_v1alpha1_FulcioService(&in.Fulcio, &out.Fulcio, s); err != nil {
		return err
	}
	if err := Convert_v1_ServiceReference_To_v1alpha1_RekorService(&in.Rekor, &out.Rekor, s); err != nil {
		return err
	}
	if err := Convert_v1_ServiceReference_To_v1alpha1_TsaService(&in.Tsa, &out.Tsa, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1alpha1_TufPvc_To_v1_Pvc(in *TufPvc, out *rhtasv1.Pvc, s apiconversion.Scope) error {
	pvc := Pvc(*in)
	return Convert_v1alpha1_Pvc_To_v1_Pvc(&pvc, out, s)
}

func Convert_v1_Pvc_To_v1alpha1_TufPvc(in *rhtasv1.Pvc, out *TufPvc, s apiconversion.Scope) error {
	var pvc Pvc
	if err := Convert_v1_Pvc_To_v1alpha1_Pvc(in, &pvc, s); err != nil {
		return err
	}
	*out = TufPvc(pvc)
	return nil
}
