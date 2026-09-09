package v1alpha1

import (
	"net/url"
	"slices"

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

	hasDataToConvert := in.TreeID != nil || in.PrivateKeyRef != nil || in.PublicKeyRef != nil || len(in.RootCertificates) > 0
	if !hasDataToConvert {
		return nil
	}

	activeIdx := slices.IndexFunc(out.Logs, func(l rhtasv1.CTlogLogStatus) bool { return l.Active })
	idx := slices.IndexFunc(out.Logs, func(l rhtasv1.CTlogLogStatus) bool { return l.Prefix == v1alpha1Prefix })
	var log *rhtasv1.CTlogLogStatus

	if idx != -1 {
		log = &out.Logs[idx]
	} else {
		out.Logs = append(out.Logs, rhtasv1.CTlogLogStatus{
			Prefix: v1alpha1Prefix,
		})
		log = &out.Logs[len(out.Logs)-1]
	}

	log.LogId = in.TreeID
	if activeIdx == -1 || activeIdx == idx {
		log.Active = true
	}

	if in.PrivateKeyRef != nil {
		log.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, log.PrivateKeyRef, s); err != nil {
			return err
		}
	}

	if in.PrivateKeyPasswordRef != nil {
		log.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyPasswordRef, log.PrivateKeyPasswordRef, s); err != nil {
			return err
		}
	}

	if in.PublicKeyRef != nil {
		log.PublicKeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PublicKeyRef, log.PublicKeyRef, s); err != nil {
			return err
		}
	}

	if len(in.RootCertificates) > 0 {
		log.RootCertificates = make([]rhtasv1.SecretKeySelector, len(in.RootCertificates))
		for i, root := range in.RootCertificates {
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(&root, &log.RootCertificates[i], s); err != nil {
				return err
			}
		}
	}

	return nil
}

func Convert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in *rhtasv1.CTlogStatus, out *CTlogStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1_CTlogStatus_To_v1alpha1_CTlogStatus(in, out, s); err != nil {
		return err
	}
	out.Url = in.URL
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
			if log.PrivateKeyPasswordRef != nil {
				out.PrivateKeyPasswordRef = &SecretKeySelector{}
				if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(log.PrivateKeyPasswordRef, out.PrivateKeyPasswordRef, s); err != nil {
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
			out.TreeID = log.LogId
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

	hasDataToConvert := in.TreeID != nil || in.PrivateKeyRef != nil || in.PublicKeyRef != nil || len(in.RootCertificates) > 0
	if !hasDataToConvert {
		return nil
	}

	activeIdx := slices.IndexFunc(out.Logs, func(l rhtasv1.CTLogConfig) bool { return ptr.Deref(l.Active, false) })
	idx := slices.IndexFunc(out.Logs, func(l rhtasv1.CTLogConfig) bool { return l.Prefix == v1alpha1Prefix })
	var log *rhtasv1.CTLogConfig

	if idx != -1 {
		log = &out.Logs[idx]
	} else {
		out.Logs = append(out.Logs, rhtasv1.CTLogConfig{
			Prefix: v1alpha1Prefix,
		})
		log = &out.Logs[len(out.Logs)-1]
	}

	log.LogId = in.TreeID
	log.Signer = &rhtasv1.CTlogSigner{
		Type: rhtasv1.SignerTypeFile,
	}
	if activeIdx == -1 || activeIdx == idx {
		log.Active = ptr.To(true)
	}
	if in.PrivateKeyRef != nil || in.PublicKeyRef != nil {
		log.Signer.File = &rhtasv1.CTlogFile{}
	}

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

func restore_v1_CTlog_spec(dst *rhtasv1.CTlogSpec, restored *rhtasv1.CTlogSpec) error {
	dst.ImagePullSecrets = restored.ImagePullSecrets
	dst.TrustedCA = restored.TrustedCA
	dst.Monitoring.ServiceMonitor = restored.Monitoring.ServiceMonitor
	dst.Monitoring.Tuf.Ref = restored.Monitoring.Tuf.Ref
	dst.PodExtensions = restored.PodExtensions
	dst.Auth = restored.Auth
	dst.Ingress = restored.Ingress
	dst.Fulcio = restored.Fulcio
	dst.Trillian.Ref = restored.Trillian.Ref

	// Restore spec logs
	dstIdx := slices.IndexFunc(dst.Logs, func(l rhtasv1.CTLogConfig) bool { return l.Prefix == v1alpha1Prefix })
	for _, rLog := range restored.Logs {
		if rLog.Prefix != v1alpha1Prefix || dstIdx == -1 {
			dst.Logs = append(dst.Logs, rLog)
		} else {
			dstLog := &dst.Logs[dstIdx]
			dstLog.Active = rLog.Active
			dstLog.FrozenSTH = rLog.FrozenSTH
			dstLog.Readonly = rLog.Readonly
			dstLog.Mirror = rLog.Mirror
			dstLog.NotAfterStart = rLog.NotAfterStart
			dstLog.NotAfterLimit = rLog.NotAfterLimit
			if rLog.Signer != nil {
				if dstLog.Signer == nil {
					dstLog.Signer = rLog.Signer
				} else {
					dstLog.Signer.PKCS11 = rLog.Signer.PKCS11
					dstLog.Signer.Type = rLog.Signer.Type
				}
			}
		}
	}
	return nil
}

func (src *CTlog) ConvertTo(dstRaw conversion.Hub) error { //nolint:gocyclo
	dst := dstRaw.(*rhtasv1.CTlog)
	if err := Convert_v1alpha1_CTlog_To_v1_CTlog(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.CTlog{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	if err := restore_v1_CTlog_spec(&dst.Spec, &restored.Spec); err != nil {
		return err
	}

	// Restore status
	dstStatusIdx := slices.IndexFunc(dst.Status.Logs, func(l rhtasv1.CTlogLogStatus) bool { return l.Prefix == v1alpha1Prefix })
	for _, rLog := range restored.Status.Logs {
		if rLog.Prefix != v1alpha1Prefix || dstStatusIdx == -1 {
			dst.Status.Logs = append(dst.Status.Logs, rLog)
		} else {
			dstLog := &dst.Status.Logs[dstStatusIdx]
			dstLog.PublicKey = rLog.PublicKey
			dstLog.SignerType = rLog.SignerType
		}
	}

	if dst.Status.URL != "" && len(dst.Spec.Logs) > 0 {
		// Find the active log, or use the first one if none are explicitly marked active
		activeLog := dst.Spec.Logs[0]
		for _, log := range dst.Spec.Logs {
			if log.Active != nil && *log.Active {
				activeLog = log
				break
			}
		}
		u, err := url.Parse(dst.Status.URL)
		if err != nil {
			return err
		}
		u.Path = "/" + activeLog.Prefix
		dst.Status.URL = u.String()
	}

	return nil
}

func (dst *CTlog) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.CTlog)
	if err := Convert_v1_CTlog_To_v1alpha1_CTlog(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
