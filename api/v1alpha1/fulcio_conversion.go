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
	if err := Convert_v1alpha1_FulcioCert_To_v1_FulcioSigner(&in.Certificate, &out.Signer, s); err != nil {
		return err
	}
	return Convert_v1alpha1_ExternalAccess_To_v1_Ingress(&in.ExternalAccess, &out.Ingress, s)
}

func Convert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in *rhtasv1.FulcioSpec, out *FulcioSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_FulcioSpec_To_v1alpha1_FulcioSpec(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1_FulcioSigner_To_v1alpha1_FulcioCert(&in.Signer, &out.Certificate, s); err != nil {
		return err
	}
	return Convert_v1_Ingress_To_v1alpha1_ExternalAccess(&in.Ingress, &out.ExternalAccess, s)
}

func Convert_v1alpha1_FulcioCert_To_v1_FulcioSigner(in *FulcioCert, out *rhtasv1.FulcioSigner, s apiconversion.Scope) error {
	out.Type = rhtasv1.SignerTypeFile
	out.CertificateChain.CommonName = in.CommonName
	out.CertificateChain.OrganizationName = in.OrganizationName
	out.CertificateChain.OrganizationEmail = in.OrganizationEmail
	if in.CARef != nil {
		out.CertificateChain.CertificateChainRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.CARef, out.CertificateChain.CertificateChainRef, s); err != nil {
			return err
		}
	}
	if in.PrivateKeyRef != nil || in.PrivateKeyPasswordRef != nil { //nolint:staticcheck
		out.File = &rhtasv1.FulcioFile{}
		if in.PrivateKeyRef != nil {
			out.File.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, out.File.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.PrivateKeyPasswordRef != nil { //nolint:staticcheck
			out.File.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{}                                                                                   //nolint:staticcheck
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyPasswordRef, out.File.PrivateKeyPasswordRef, s); err != nil { //nolint:staticcheck
				return err
			}
		}
	}
	return nil
}

func Convert_v1_FulcioSigner_To_v1alpha1_FulcioCert(in *rhtasv1.FulcioSigner, out *FulcioCert, s apiconversion.Scope) error {
	out.CommonName = in.CertificateChain.CommonName
	out.OrganizationName = in.CertificateChain.OrganizationName
	out.OrganizationEmail = in.CertificateChain.OrganizationEmail
	if in.CertificateChain.CertificateChainRef != nil {
		out.CARef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.CertificateChain.CertificateChainRef, out.CARef, s); err != nil {
			return err
		}
	}
	if in.File != nil {
		if in.File.PrivateKeyRef != nil {
			out.PrivateKeyRef = &SecretKeySelector{}
			if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.File.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.File.PrivateKeyPasswordRef != nil { //nolint:staticcheck
			out.PrivateKeyPasswordRef = &SecretKeySelector{}                                                                                                //nolint:staticcheck
			if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.File.PrivateKeyPasswordRef, out.PrivateKeyPasswordRef, s); err != nil { //nolint:staticcheck
				return err
			}
		}
	}
	return nil
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
	dst.Spec.Signer.Type = restored.Spec.Signer.Type
	if dst.Spec.Signer.File == nil {
		dst.Spec.Signer.File = restored.Spec.Signer.File
	}
	if dst.Spec.Signer.Kms == nil {
		dst.Spec.Signer.Kms = restored.Spec.Signer.Kms
	}
	dst.Status.CertificateChain = restored.Status.CertificateChain
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor

	// v1alpha1 inject prefix into URL - we need to restore empty URL to allow ref resolution
	if restored.Spec.Ctlog.URL == "" && dst.Spec.Ctlog.URL == "///trusted-artifact-signer" { //nolint:goconst
		dst.Spec.Ctlog.URL = ""
	}
	if dst.Spec.Ctlog.URL == "" {
		dst.Spec.Ctlog.Ref = restored.Spec.Ctlog.Ref
	}
	dst.Spec.PodExtensions = restored.Spec.PodExtensions
	dst.Spec.Auth = restored.Spec.Auth
	return nil
}

func (dst *Fulcio) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Fulcio)
	if err := Convert_v1_Fulcio_To_v1alpha1_Fulcio(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
