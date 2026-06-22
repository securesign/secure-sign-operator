package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

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
	dst.Spec.Ctlog.ImagePullSecrets = restored.Spec.Ctlog.ImagePullSecrets
	dst.Spec.Ctlog.TrustedCA = restored.Spec.Ctlog.TrustedCA
	dst.Spec.Rekor.ImagePullSecrets = restored.Spec.Rekor.ImagePullSecrets
	dst.Spec.Trillian.ImagePullSecrets = restored.Spec.Trillian.ImagePullSecrets
	dst.Spec.Tuf.ImagePullSecrets = restored.Spec.Tuf.ImagePullSecrets
	if dst.Spec.TimestampAuthority != nil && restored.Spec.TimestampAuthority != nil {
		dst.Spec.TimestampAuthority.ImagePullSecrets = restored.Spec.TimestampAuthority.ImagePullSecrets
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
