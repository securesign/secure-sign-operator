package v1alpha1

import (
	"net/url"

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
	out.URL = in.Url
	// Only populate v1 status logs if v1alpha1 has actual status data to convert.
	// v1alpha1 deprecated fields (TreeID, PrivateKeyRef, PublicKeyRef, RootCertificates) need to be
	// converted to v1 Status.Logs. Find or create a log with the standard v1alpha1 prefix.
	if in.TreeID == nil && in.PrivateKeyRef == nil && in.PublicKeyRef == nil && len(in.RootCertificates) == 0 {
		// No v1alpha1 status fields to convert, don't create a log
		return nil
	}
	idx := -1
	for i := range out.Logs {
		if out.Logs[i].Prefix == v1alpha1Prefix {
			idx = i
			break
		}
	}
	if idx == -1 {
		out.Logs = append(out.Logs, rhtasv1.CTlogLogStatus{Prefix: v1alpha1Prefix})
		idx = len(out.Logs) - 1
	}
	log := &out.Logs[idx]
	if in.TreeID != nil {
		log.LogId = in.TreeID
	}
	if in.PrivateKeyRef != nil {
		log.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
		if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, log.PrivateKeyRef, s); err != nil {
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
	// Find or create the v1alpha1-prefix log. Even if v1alpha1 has no deprecated fields to project,
	// a valid active v1 log may intentionally omit tree ID, signer, and roots because the controller
	// resolves them automatically. We must not delete such logs during conversion.
	idx := -1
	for i := range out.Logs {
		if out.Logs[i].Prefix == v1alpha1Prefix {
			idx = i
			break
		}
	}

	// Only create a new log entry if v1alpha1 has actual data to convert
	hasDataToConvert := in.TreeID != nil || in.PrivateKeyRef != nil || in.PublicKeyRef != nil || len(in.RootCertificates) > 0

	if idx == -1 && hasDataToConvert {
		// Check if any existing log is already marked as active
		hasActiveLog := false
		for _, log := range out.Logs {
			if log.Active != nil && *log.Active {
				hasActiveLog = true
				break
			}
		}
		// Only set this log as active if no other log is currently active
		out.Logs = append(out.Logs, rhtasv1.CTLogConfig{
			Prefix: v1alpha1Prefix,
			Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
			Active: ptr.To(!hasActiveLog),
		})
		idx = len(out.Logs) - 1
	}

	// Only update the log if we have data to convert or if the log already exists
	if idx != -1 {
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
	}
	return nil
}

func (src *CTlog) ConvertTo(dstRaw conversion.Hub) error { //nolint:gocyclo
	dst := dstRaw.(*rhtasv1.CTlog)
	if err := Convert_v1alpha1_CTlog_To_v1_CTlog(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.CTlog{}
	hasConversionData, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}
	// If UnmarshalData returns false (no conversion-data annotation), it means this is a legacy object.
	// This is normal for objects that predate the v1 storage format.
	// We'll apply legacy handlers below if needed.
	// Restore v1-only Spec fields from storage (fields that don't exist in v1alpha1)
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Spec.TrustedCA = restored.Spec.TrustedCA
	// Merge restored logs with converted logs: keep all v1-only logs (non-v1alpha1-prefix)
	// and preserve the converted "trusted-artifact-signer" log from the conversion above.
	// Preserve the original active log selection: if a v1-only log was active before,
	// keep it active and deactivate the converted v1alpha1 log.
	// If v1alpha1 has no deprecated fields to project, preserve the stored v1alpha1 log unchanged.
	for _, rlog := range restored.Spec.Logs {
		if rlog.Prefix != v1alpha1Prefix {
			// This is a v1-only log (not the legacy v1alpha1 log), append it
			dst.Spec.Logs = append(dst.Spec.Logs, rlog)
			// If this restored log was the original active log, deactivate the v1alpha1 log
			if rlog.Active != nil && *rlog.Active {
				for i := range dst.Spec.Logs {
					if dst.Spec.Logs[i].Prefix == v1alpha1Prefix {
						dst.Spec.Logs[i].Active = ptr.To(false)
						break
					}
				}
			}
		}
	}
	// If src.Spec has no deprecated fields and a "trusted-artifact-signer" log exists in restored,
	// overlay editable v1alpha1 fields onto the restored log rather than using the auto-converted one.
	// This preserves a valid v1 log that intentionally omits auto-resolved fields.
	if src.Spec.TreeID == nil && src.Spec.PrivateKeyRef == nil && src.Spec.PublicKeyRef == nil && len(src.Spec.RootCertificates) == 0 {
		// v1alpha1 has no deprecated fields to project. Check if restored has the legacy log.
		for _, rlog := range restored.Spec.Logs {
			if rlog.Prefix == v1alpha1Prefix {
				// Find and replace the auto-converted log with the restored one
				for i := range dst.Spec.Logs {
					if dst.Spec.Logs[i].Prefix == v1alpha1Prefix {
						dst.Spec.Logs[i] = rlog
						break
					}
				}
				break
			}
		}
	}
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
	// Restore v1-only Status fields from storage (Status.Logs doesn't exist in v1alpha1)
	// Merge restored status logs with converted logs: keep all v1-only logs (non-v1alpha1-prefix)
	// and preserve the converted "trusted-artifact-signer" log from the conversion above.
	for _, rlog := range restored.Status.Logs {
		if rlog.Prefix != v1alpha1Prefix {
			// This is a v1-only log (not the legacy v1alpha1 log), append it
			dst.Status.Logs = append(dst.Status.Logs, rlog)
		}
	}
	// Restore important status fields from storage for backward compatibility.
	// Match each dst status log with its corresponding restored status log by prefix.
	for i := range dst.Status.Logs {
		for _, rlog := range restored.Status.Logs {
			if rlog.Prefix == dst.Status.Logs[i].Prefix {
				// Restore password refs for backward compatibility with encrypted keys
				dst.Status.Logs[i].PrivateKeyPasswordRef = rlog.PrivateKeyPasswordRef
				// Restore already-resolved public key data (needed for TUF trust material)
				if dst.Status.Logs[i].PublicKey == "" {
					dst.Status.Logs[i].PublicKey = rlog.PublicKey
				}
				// Restore signer type if not already set
				if dst.Status.Logs[i].SignerType == "" {
					dst.Status.Logs[i].SignerType = rlog.SignerType
				}
				break
			}
		}
	}
	// Also restore v1alpha1Prefix log's status fields if present in storage
	for i := range dst.Status.Logs {
		if dst.Status.Logs[i].Prefix == v1alpha1Prefix {
			for _, rlog := range restored.Status.Logs {
				if rlog.Prefix == v1alpha1Prefix {
					// Restore password refs for backward compatibility
					if dst.Status.Logs[i].PrivateKeyPasswordRef == nil {
						dst.Status.Logs[i].PrivateKeyPasswordRef = rlog.PrivateKeyPasswordRef
					}
					// Restore already-resolved public key (critical for TUF trust material)
					if dst.Status.Logs[i].PublicKey == "" {
						dst.Status.Logs[i].PublicKey = rlog.PublicKey
					}
					// Restore signer type if not already set
					if dst.Status.Logs[i].SignerType == "" {
						dst.Status.Logs[i].SignerType = rlog.SignerType
					}
					break
				}
			}
			break
		}
	}
	// For legacy objects with empty Spec but with operational Status, create minimal entries.
	// This preserves signer keys that were auto-generated during years of operation.
	// Without matching spec entries, align_status_logs would delete the status data,
	// causing keys to be regenerated (data loss and signature invalidation during upgrade).
	// Only apply this for objects without conversion-data (true legacy objects).
	// Legacy objects may have either Status.Logs (if status conversion succeeded) or just Status.Url.
	if !hasConversionData && len(dst.Spec.Logs) == 0 && (len(dst.Status.Logs) > 0 || dst.Status.Url != "") {
		// If Status.Logs is already populated from status conversion, use it to create Spec entries
		if len(dst.Status.Logs) > 0 {
			for _, statusLog := range dst.Status.Logs {
				dst.Spec.Logs = append(dst.Spec.Logs, rhtasv1.CTLogConfig{
					Prefix: statusLog.Prefix,
					Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
					Active: ptr.To(statusLog.Active),
				})
			}
		} else if dst.Status.Url != "" {
			// Legacy object has URL but no per-log status data. Create a default entry.
			// Use the v1alpha1 prefix convention since this is a legacy object.
			dst.Spec.Logs = append(dst.Spec.Logs, rhtasv1.CTLogConfig{
				Prefix: v1alpha1Prefix,
				Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
				Active: ptr.To(true),
			})
			// Create a matching Status.Logs entry to hold auto-generated signer data
			dst.Status.Logs = append(dst.Status.Logs, rhtasv1.CTlogLogStatus{
				Prefix: v1alpha1Prefix,
				Active: true,
			})
		}
	}
	// Shared Status fields (Conditions, ServerConfigRef, Tls, Url) are properly converted by
	// autoConvert_v1alpha1_CTlog_To_v1_CTlog above. Do not restore them from storage.
	// However, reconstruct Status.Url to include the active log prefix (which v1alpha1 strips)
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
