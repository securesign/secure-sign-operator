package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in *rhtasv1.FulcioStatus, out *FulcioStatus, s apiconversion.Scope) error {
	return autoConvert_v1_FulcioStatus_To_v1alpha1_FulcioStatus(in, out, s)
}

func Convert_v1alpha1_FulcioSpec_To_v1_FulcioSpec(in *FulcioSpec, out *rhtasv1.FulcioSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_FulcioSpec_To_v1_FulcioSpec(in, out, s); err != nil {
		return err
	}
	return Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s)
}

func Convert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in *rhtasv1.FulcioSpec, out *FulcioSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in, out, s); err != nil {
		return err
	}
	return Convert_v1_Ingress_To_v1alpha1_ExternalAccess(&in.Ingress, &out.ExternalAccess, s)
}

func Convert_v1alpha1_FulcioCert_To_v1_FulcioCertStatus(in *FulcioCert, out *rhtasv1.FulcioCertStatus, s apiconversion.Scope) error {
	if in.PrivateKeyRef != nil {
		out.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
			return err
		}
	}
	if in.PrivateKeyPasswordRef != nil {
		out.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyPasswordRef, out.PrivateKeyPasswordRef, s); err != nil {
			return err
		}
	}
	if in.CARef != nil {
		out.CARef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.CARef, out.CARef, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1_FulcioCertStatus_To_v1alpha1_FulcioCert(in *rhtasv1.FulcioCertStatus, out *FulcioCert, s apiconversion.Scope) error {
	if in.PrivateKeyRef != nil {
		out.PrivateKeyRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
			return err
		}
	}
	if in.PrivateKeyPasswordRef != nil {
		out.PrivateKeyPasswordRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.PrivateKeyPasswordRef, out.PrivateKeyPasswordRef, s); err != nil {
			return err
		}
	}
	if in.CARef != nil {
		out.CARef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.CARef, out.CARef, s); err != nil {
			return err
		}
	}
	return nil
}

func (src *Fulcio) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Fulcio)
	if err := Convert_v1alpha1_Fulcio_To_v1_Fulcio(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Fulcio{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Status.CertificateChain = restored.Status.CertificateChain
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	return nil
}

func (dst *Fulcio) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Fulcio)
	if err := Convert_v1_Fulcio_To_v1alpha1_Fulcio(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
