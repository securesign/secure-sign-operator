package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1alpha1_TimestampAuthoritySigner_To_v1_TimestampAuthoritySignerStatus(in *TimestampAuthoritySigner, out *rhtasv1.TimestampAuthoritySignerStatus, s apiconversion.Scope) error {
	if in.CertificateChain.CertificateChainRef != nil {
		out.CertificateChainRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.CertificateChain.CertificateChainRef, out.CertificateChainRef, s); err != nil {
			return err
		}
	}
	if in.File != nil {
		out.FileSigner = &rhtasv1.FileSignerStatus{}
		if in.File.PrivateKeyRef != nil {
			out.FileSigner.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.File.PrivateKeyRef, out.FileSigner.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.File.PasswordRef != nil {
			out.FileSigner.PasswordRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.File.PasswordRef, out.FileSigner.PasswordRef, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1_TimestampAuthoritySignerStatus_To_v1alpha1_TimestampAuthoritySigner(in *rhtasv1.TimestampAuthoritySignerStatus, out *TimestampAuthoritySigner, s apiconversion.Scope) error {
	if in.CertificateChainRef != nil {
		out.CertificateChain.CertificateChainRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.CertificateChainRef, out.CertificateChain.CertificateChainRef, s); err != nil {
			return err
		}
	}
	if in.FileSigner != nil {
		out.File = &File{}
		if in.FileSigner.PrivateKeyRef != nil {
			out.File.PrivateKeyRef = &SecretKeySelector{}
			if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.FileSigner.PrivateKeyRef, out.File.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.FileSigner.PasswordRef != nil {
			out.File.PasswordRef = &SecretKeySelector{}
			if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.FileSigner.PasswordRef, out.File.PasswordRef, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1alpha1_TimestampAuthorityStatus_To_v1_TimestampAuthorityStatus(in *TimestampAuthorityStatus, out *rhtasv1.TimestampAuthorityStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_TimestampAuthorityStatus_To_v1_TimestampAuthorityStatus(in, out, s); err != nil {
		return err
	}
	if in.NTPMonitoring != nil && in.NTPMonitoring.Config != nil && in.NTPMonitoring.Config.NtpConfigRef != nil {
		out.NtpConfigRef = &rhtasv1.LocalObjectReference{Name: in.NTPMonitoring.Config.NtpConfigRef.Name}
	}
	return nil
}

func Convert_v1_TimestampAuthorityStatus_To_v1alpha1_TimestampAuthorityStatus(in *rhtasv1.TimestampAuthorityStatus, out *TimestampAuthorityStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1_TimestampAuthorityStatus_To_v1alpha1_TimestampAuthorityStatus(in, out, s); err != nil {
		return err
	}
	if in.NtpConfigRef != nil {
		out.NTPMonitoring = &NTPMonitoring{
			Config: &NtpMonitoringConfig{
				NtpConfigRef: &LocalObjectReference{Name: in.NtpConfigRef.Name},
			},
		}
	}
	return nil
}

func Convert_v1alpha1_TsaCertificateAuthority_To_v1_TsaCertificateAuthority(in *TsaCertificateAuthority, out *rhtasv1.TsaCertificateAuthority, _ apiconversion.Scope) error {
	out.CommonName = in.CommonName
	out.OrganizationName = in.OrganizationName
	out.OrganizationEmail = in.OrganizationEmail
	return nil
}

func Convert_v1_TsaCertificateAuthority_To_v1alpha1_TsaCertificateAuthority(in *rhtasv1.TsaCertificateAuthority, out *TsaCertificateAuthority, _ apiconversion.Scope) error {
	out.CommonName = in.CommonName
	out.OrganizationName = in.OrganizationName
	out.OrganizationEmail = in.OrganizationEmail
	return nil
}

func (src *TimestampAuthority) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.TimestampAuthority)
	if err := Convert_v1alpha1_TimestampAuthority_To_v1_TimestampAuthority(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.TimestampAuthority{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	return nil
}

func (dst *TimestampAuthority) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.TimestampAuthority)
	if err := Convert_v1_TimestampAuthority_To_v1alpha1_TimestampAuthority(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
