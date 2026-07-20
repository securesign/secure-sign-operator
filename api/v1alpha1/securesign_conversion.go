package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1alpha1_SecuresignTSAStatus_To_v1_SecuresignTSAStatus(in *SecuresignTSAStatus, out *rhtasv1.SecuresignTSAStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_SecuresignTSAStatus_To_v1_SecuresignTSAStatus(in, out, s); err != nil {
		return err
	}
	if out.Url != "" {
		var err error
		if out.Url, err = urlWithPath(out.Url, rhtasv1.TimestampPath); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1_SecuresignTSAStatus_To_v1alpha1_SecuresignTSAStatus(in *rhtasv1.SecuresignTSAStatus, out *SecuresignTSAStatus, s apiconversion.Scope) error {
	if err := autoConvert_v1_SecuresignTSAStatus_To_v1alpha1_SecuresignTSAStatus(in, out, s); err != nil {
		return err
	}
	if out.Url != "" {
		var err error
		if out.Url, err = urlWithoutPath(out.Url); err != nil {
			return err
		}
	}
	return nil
}

func (src *Securesign) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Securesign)
	if err := Convert_v1alpha1_Securesign_To_v1_Securesign(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Securesign{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.Fulcio.ImagePullSecrets = restored.Spec.Fulcio.ImagePullSecrets
	dst.Spec.Fulcio.Monitoring.ServiceMonitor = restored.Spec.Fulcio.Monitoring.ServiceMonitor
	dst.Spec.Ctlog.ImagePullSecrets = restored.Spec.Ctlog.ImagePullSecrets
	dst.Spec.Ctlog.TrustedCA = restored.Spec.Ctlog.TrustedCA
	dst.Spec.Ctlog.Monitoring.ServiceMonitor = restored.Spec.Ctlog.Monitoring.ServiceMonitor
	dst.Spec.Ctlog.Prefix = restored.Spec.Ctlog.Prefix
	dst.Spec.Rekor.ImagePullSecrets = restored.Spec.Rekor.ImagePullSecrets
	dst.Spec.Rekor.Monitoring.ServiceMonitor = restored.Spec.Rekor.Monitoring.ServiceMonitor
	dst.Spec.Trillian.ImagePullSecrets = restored.Spec.Trillian.ImagePullSecrets
	dst.Spec.Trillian.Monitoring.ServiceMonitor = restored.Spec.Trillian.Monitoring.ServiceMonitor
	dst.Spec.Tuf.ImagePullSecrets = restored.Spec.Tuf.ImagePullSecrets
	dst.Spec.Tuf.TrustedCA = restored.Spec.Tuf.TrustedCA
	if dst.Spec.TimestampAuthority != nil && restored.Spec.TimestampAuthority != nil {
		dst.Spec.TimestampAuthority.ImagePullSecrets = restored.Spec.TimestampAuthority.ImagePullSecrets
		dst.Spec.TimestampAuthority.Monitoring.ServiceMonitor = restored.Spec.TimestampAuthority.Monitoring.ServiceMonitor
		// restore also the auth from annotation for case where no KMS or Tink is set
		dst.Spec.TimestampAuthority.Signer.Auth = mergeAuths(dst.Spec.TimestampAuthority.Signer.Auth, restored.Spec.TimestampAuthority.Signer.Auth)
	}
	return nil
}

func (dst *Securesign) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Securesign)
	if err := Convert_v1_Securesign_To_v1alpha1_Securesign(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
