package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

const v1alpha1Prefix = "trusted-artifact-signer"

func Convert_v1alpha1_CTlogStatus_To_v1_CTlogStatus(in *CTlogStatus, out *rhtasv1.CTlogStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_CTlogStatus_To_v1_CTlogStatus(in, out, s); err != nil {
		return err
	}
	// v1alpha1 deprecated fields are converted to v1 Status.Logs by ConvertFrom
	return nil
}

func Convert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in *rhtasv1.CTlogStatus, out *CTlogStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in, out, s); err != nil {
		return err
	}
	if out.Url != "" {
		var err error
		if out.Url, _, err = splitURLPath(out.Url); err != nil {
			return err
		}
	}
	// Derive deprecated status fields from the active log in Status.Logs
	for _, log := range in.Logs {
		if log.Prefix == v1alpha1Prefix {
			if log.LogId != nil {
				out.TreeID = log.LogId
			}
			if log.PrivateKeyRef != nil {
				out.PrivateKeyRef = &SecretKeySelector{}
				if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(log.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
					return err
				}
			}
			if log.PublicKeyRef != nil {
				out.PublicKeyRef = &SecretKeySelector{}
				if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(log.PublicKeyRef, out.PublicKeyRef, s); err != nil {
					return err
				}
			}
			if len(log.RootCertificates) > 0 {
				out.RootCertificates = make([]SecretKeySelector, len(log.RootCertificates))
				for i, root := range log.RootCertificates {
					if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(&root, &out.RootCertificates[i], s); err != nil {
						return err
					}
				}
			}
			break
		}
	}
	return nil
}

func Convert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in *rhtasv1.CTlogSpec, out *CTlogSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in, out, s); err != nil {
		return err
	}
	// Extract signer config, LogId, and RootCertificates from log with "trusted-artifact-signer" prefix
	// v1alpha1 hardcodes this prefix, so we look for it in the v1 Logs array
	for _, log := range in.Logs {
		if log.Prefix == v1alpha1Prefix {
			if log.LogId != nil {
				out.TreeID = log.LogId
			}
			if log.Signer != nil && log.Signer.File != nil {
				if log.Signer.File.PrivateKeyRef != nil {
					out.PrivateKeyRef = &SecretKeySelector{}
					if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(log.Signer.File.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
						return err
					}
				}
				if log.Signer.File.PublicKeyRef != nil {
					out.PublicKeyRef = &SecretKeySelector{}
					if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(log.Signer.File.PublicKeyRef, out.PublicKeyRef, s); err != nil {
						return err
					}
				}
			}
			// Extract root certificates from the log
			if len(log.RootCerts) > 0 {
				out.RootCertificates = make([]SecretKeySelector, len(log.RootCerts))
				for i, root := range log.RootCerts {
					if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(&root, &out.RootCertificates[i], s); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	return nil
}

func Convert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in *CTlogSpec, out *rhtasv1.CTlogSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in, out, s); err != nil {
		return err
	}
	// v1alpha1 always uses the hardcoded "trusted-artifact-signer" prefix.
	// Find the matching log by prefix, or append a new entry if not found.
	idx := -1
	for i := range out.Logs {
		if out.Logs[i].Prefix == v1alpha1Prefix {
			idx = i
			break
		}
	}
	if idx == -1 {
		out.Logs = append(out.Logs, rhtasv1.CTLogConfig{
			Prefix: v1alpha1Prefix,
			Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
			Active: ptr.To(true),
		})
		idx = len(out.Logs) - 1
	}
	log := &out.Logs[idx]
	if in.TreeID != nil {
		log.LogId = in.TreeID
	}
	if in.PrivateKeyRef != nil || in.PublicKeyRef != nil {
		if log.Signer == nil {
			log.Signer = &rhtasv1.CTlogSigner{}
		}
		log.Signer.Type = rhtasv1.SignerTypeFile
		log.Signer.File = &rhtasv1.CTlogFile{}
		if in.PrivateKeyRef != nil {
			log.Signer.File.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, log.Signer.File.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.PublicKeyRef != nil {
			log.Signer.File.PublicKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PublicKeyRef, log.Signer.File.PublicKeyRef, s); err != nil {
				return err
			}
		}
	}
	if len(in.RootCertificates) > 0 {
		log.RootCerts = make([]rhtasv1.SecretKeySelector, len(in.RootCertificates))
		for i, root := range in.RootCertificates {
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(&root, &log.RootCerts[i], s); err != nil {
				return err
			}
		}
	}
	return nil
}

func (src *CTlog) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.CTlog)
	if err := Convert_v1alpha1_CTlog_To_v1_CTlog(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.CTlog{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	// Restore v1-only Spec fields from storage (fields that don't exist in v1alpha1)
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Spec.TrustedCA = restored.Spec.TrustedCA
	dst.Spec.Logs = restored.Spec.Logs
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	if dst.Spec.Trillian.URL == "" {
		dst.Spec.Trillian.Ref = restored.Spec.Trillian.Ref
	}
	dst.Spec.Fulcio = restored.Spec.Fulcio
	if dst.Spec.Monitoring.Tuf.URL == "" {
		dst.Spec.Monitoring.Tuf.Ref = restored.Spec.Monitoring.Tuf.Ref
	}
	dst.Spec.PodExtensions = restored.Spec.PodExtensions
	dst.Spec.Auth = restored.Spec.Auth
	dst.Spec.Ingress = restored.Spec.Ingress
	// All Status fields (Conditions, ServerConfigRef, Tls, Url) are shared between v1 and v1alpha1
	// and are properly converted by autoConvert_v1alpha1_CTlog_To_v1_CTlog above.
	// Do not restore Status from storage.
	return nil
}

func (dst *CTlog) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.CTlog)
	if err := Convert_v1_CTlog_To_v1alpha1_CTlog(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
