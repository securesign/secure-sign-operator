package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *TimestampAuthority) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.TimestampAuthority)
	if err := Convert_v1alpha1_TimestampAuthority_To_v1_TimestampAuthority(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.TimestampAuthority{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	return nil
}

func (dst *TimestampAuthority) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.TimestampAuthority)
	if err := Convert_v1_TimestampAuthority_To_v1alpha1_TimestampAuthority(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
