package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"k8s.io/apimachinery/pkg/api/equality"
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
	if in.Signer.File == nil {
		return nil
	}
	if in.Signer.File.PrivateKeyRef != nil {
		out.PrivateKeyRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.Signer.File.PrivateKeyRef, out.PrivateKeyRef, s); err != nil {
			return err
		}
	}
	if in.Signer.File.PrivateKeyPasswordRef != nil { //nolint:staticcheck
		out.PrivateKeyPasswordRef = &SecretKeySelector{}                                                                                                       //nolint:staticcheck
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.Signer.File.PrivateKeyPasswordRef, out.PrivateKeyPasswordRef, s); err != nil { //nolint:staticcheck
			return err
		}
	}
	if in.Signer.File.PublicKeyRef != nil {
		out.PublicKeyRef = &SecretKeySelector{}
		if err := Convert_v1_SecretKeySelector_To_v1alpha1_SecretKeySelector(in.Signer.File.PublicKeyRef, out.PublicKeyRef, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in *CTlogSpec, out *rhtasv1.CTlogSpec, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_CTlogSpec_To_v1_CTlogSpec(in, out, s); err != nil {
		return err
	}
	out.Signer.Type = rhtasv1.CTlogSignerTypeFile
	if in.PrivateKeyRef != nil || in.PrivateKeyPasswordRef != nil || in.PublicKeyRef != nil { //nolint:staticcheck
		out.Signer.File = &rhtasv1.CTlogFile{}
		if in.PrivateKeyRef != nil {
			out.Signer.File.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyRef, out.Signer.File.PrivateKeyRef, s); err != nil {
				return err
			}
		}
		if in.PrivateKeyPasswordRef != nil { //nolint:staticcheck
			out.Signer.File.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{}                                                                                   //nolint:staticcheck
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PrivateKeyPasswordRef, out.Signer.File.PrivateKeyPasswordRef, s); err != nil { //nolint:staticcheck
				return err
			}
		}
		if in.PublicKeyRef != nil {
			out.Signer.File.PublicKeyRef = &rhtasv1.SecretKeySelector{}
			if err := Convert_v1alpha1_SecretKeySelector_To_v1_SecretKeySelector(in.PublicKeyRef, out.Signer.File.PublicKeyRef, s); err != nil {
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
	dst.Spec.Signer.Type = restored.Spec.Signer.Type
	// If original v1 had File=&{} (empty struct), preserve it
	if dst.Spec.Signer.File == nil && restored.Spec.Signer.File != nil {
		emptyFile := &rhtasv1.CTlogFile{}
		if equality.Semantic.DeepEqual(restored.Spec.Signer.File, emptyFile) {
			dst.Spec.Signer.File = &rhtasv1.CTlogFile{}
		}
	}
	dst.Status.PublicKey = restored.Status.PublicKey
	dst.Spec.Monitoring.ServiceMonitor = restored.Spec.Monitoring.ServiceMonitor
	dst.Spec.Prefix = restored.Spec.Prefix
	if dst.Status.Url != "" && restored.Spec.Prefix != "" {
		var err error
		if dst.Status.Url, err = buildURL(dst.Status.Url, nil, restored.Spec.Prefix); err != nil {
			return err
		}
	}
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
