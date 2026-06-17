package v1alpha1

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Fulcio) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*rhtasv1.Fulcio)
	if err := Convert_v1alpha1_Fulcio_To_v1_Fulcio(src, dst, nil); err != nil {
		return err
	}
	restored := &rhtasv1.Fulcio{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	return nil
}

func (dst *Fulcio) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*rhtasv1.Fulcio)
	if err := Convert_v1_Fulcio_To_v1alpha1_Fulcio(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}
