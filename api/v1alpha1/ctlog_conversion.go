package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

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
	return nil
}

func Convert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in *rhtasv1.CTlogSpec, out *CTlogSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1_CTlogSpec_To_v1alpha1_CTlogSpec(in, out, s); err != nil {
		return err
	}
	// Extract signer config from first active log (non-readonly)
	for _, log := range in.Logs {
		if log.Readonly == nil || !*log.Readonly {
			// This is an active log
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
			break
		}
	}
	return nil
}

func Convert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in *CTlogSpec, out *rhtasv1.CTlogSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in, out, s); err != nil {
		return err
	}
	// If we have signer keys defined, create a Logs array with a single active log
	if in.PrivateKeyRef != nil || in.PublicKeyRef != nil {
		if len(out.Logs) == 0 {
			out.Logs = make([]rhtasv1.CTLogConfig, 1)
		}
		// Populate the first log's signer config
		if out.Logs[0].Signer == nil {
			out.Logs[0].Signer = &rhtasv1.CTlogSigner{}
		}
		out.Logs[0].Signer.Type = rhtasv1.SignerTypeFile
		out.Logs[0].Signer.File = &rhtasv1.CTlogFile{}
		if in.PrivateKeyRef != nil {
			out.Logs[0].Signer.File.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, out.Logs[0].Signer.File.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.PublicKeyRef != nil {
			out.Logs[0].Signer.File.PublicKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PublicKeyRef, out.Logs[0].Signer.File.PublicKeyRef, s); err != nil {
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
	dst.Spec.ImagePullSecrets = restored.Spec.ImagePullSecrets
	dst.Spec.TrustedCA = restored.Spec.TrustedCA
	// Restore Logs array from storage
	dst.Spec.Logs = restored.Spec.Logs
	dst.Status.PublicKey = restored.Status.PublicKey
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	if dst.Spec.Trillian.URL == "" {
		dst.Spec.Trillian.Ref = restored.Spec.Trillian.Ref
	}
	if dst.Spec.Monitoring.Tuf.URL == "" {
		dst.Spec.Monitoring.Tuf.Ref = restored.Spec.Monitoring.Tuf.Ref
	}
	dst.Spec.PodExtensions = restored.Spec.PodExtensions
	dst.Spec.Auth = restored.Spec.Auth
	dst.Spec.Ingress = restored.Spec.Ingress
	return nil
}

func (dst *CTlog) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.CTlog)
	if err := Convert_v1_CTlog_To_v1alpha1_CTlog(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
