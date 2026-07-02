package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

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
	dst.Status.PublicKey = restored.Status.PublicKey
	return nil
}

func (dst *CTlog) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.CTlog)
	if err := Convert_v1_CTlog_To_v1alpha1_CTlog(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
