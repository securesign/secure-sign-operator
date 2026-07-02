package v1alpha1

import (
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	core "k8s.io/api/core/v1"
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

func Convert_v1alpha1_KMS_To_v1_KMS(in *KMS, out *rhtasv1.KMS, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_KMS_To_v1_KMS(in, out, s)
}

func Convert_v1alpha1_Tink_To_v1_Tink(in *Tink, out *rhtasv1.Tink, s apiconversion.Scope) error {
	return autoConvert_v1alpha1_Tink_To_v1_Tink(in, out, s)
}

func Convert_v1alpha1_TimestampAuthoritySigner_To_v1_TimestampAuthoritySigner(in *TimestampAuthoritySigner, out *rhtasv1.TimestampAuthoritySigner, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_TimestampAuthoritySigner_To_v1_TimestampAuthoritySigner(in, out, s); err != nil {
		return err
	}
	var auths []*rhtasv1.Auth
	if in.Kms != nil && in.Kms.Auth != nil {
		auth := new(rhtasv1.Auth)
		if err := autoConvert_v1alpha1_Auth_To_v1_Auth(in.Kms.Auth, auth, s); err != nil {
			return err
		}
		auths = append(auths, auth)
	}
	if in.Tink != nil && in.Tink.Auth != nil {
		auth := new(rhtasv1.Auth)
		if err := autoConvert_v1alpha1_Auth_To_v1_Auth(in.Tink.Auth, auth, s); err != nil {
			return err
		}
		auths = append(auths, auth)
	}
	if auth := mergeAuths(auths...); auth != nil {
		out.Auth = auth
	}
	return nil
}

func Convert_v1_TimestampAuthoritySigner_To_v1alpha1_TimestampAuthoritySigner(in *rhtasv1.TimestampAuthoritySigner, out *TimestampAuthoritySigner, s apiconversion.Scope) error {
	if err := autoConvert_v1_TimestampAuthoritySigner_To_v1alpha1_TimestampAuthoritySigner(in, out, s); err != nil {
		return err
	}
	if in.Auth != nil {
		if out.Kms != nil {
			out.Kms.Auth = new(Auth)
			if err := autoConvert_v1_Auth_To_v1alpha1_Auth(in.Auth, out.Kms.Auth, s); err != nil {
				return err
			}
		}
		if out.Tink != nil {
			out.Tink.Auth = new(Auth)
			if err := autoConvert_v1_Auth_To_v1alpha1_Auth(in.Auth, out.Tink.Auth, s); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeAuths merges the given auth objects into a single auth object and keep only unique values.
func mergeAuths(auth ...*rhtasv1.Auth) *rhtasv1.Auth {
	var merged *rhtasv1.Auth
	for _, a := range auth {
		if a == nil {
			continue
		}
		if merged == nil {
			merged = &rhtasv1.Auth{}
		}
		for _, e := range a.Env {
			if !slices.ContainsFunc(merged.Env, func(existing core.EnvVar) bool { return existing.Name == e.Name }) {
				merged.Env = append(merged.Env, e)
			}
		}
		for _, m := range a.SecretMount {
			if !slices.Contains(merged.SecretMount, m) {
				merged.SecretMount = append(merged.SecretMount, m)
			}
		}
	}
	return merged
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
	dst.Spec.Signer.Auth = mergeAuths(dst.Spec.Signer.Auth, restored.Spec.Signer.Auth)
	dst.Status.CertificateChain = restored.Status.CertificateChain
	return nil
}

func (dst *TimestampAuthority) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.TimestampAuthority)
	if err := Convert_v1_TimestampAuthority_To_v1alpha1_TimestampAuthority(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
