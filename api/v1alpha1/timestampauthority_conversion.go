package v1alpha1

import (
	v1 "github.com/securesign/operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *TimestampAuthority) ConvertTo(dstRaw conversion.Hub) error {
	if err := marshalConvert(src, dstRaw.(runtime.Object)); err != nil {
		return err
	}
	convertTSAStatusTo(src.Status, &dstRaw.(*v1.TimestampAuthority).Status)
	return nil
}

func (dst *TimestampAuthority) ConvertFrom(srcRaw conversion.Hub) error {
	if err := marshalConvert(srcRaw.(runtime.Object), dst); err != nil {
		return err
	}
	convertTSAStatusFrom(srcRaw.(*v1.TimestampAuthority).Status, &dst.Status)
	return nil
}

func convertTSAStatusTo(src TimestampAuthorityStatus, dst *v1.TimestampAuthorityStatus) {
	if src.Signer == nil {
		return
	}
	if dst.Signer == nil {
		dst.Signer = &v1.TSASignerStatus{}
	}
	if ref := src.Signer.CertificateChain.CertificateChainRef; ref != nil {
		dst.Signer.CertificateChainRef = &v1.SecretKeySelector{
			LocalObjectReference: v1.LocalObjectReference{Name: ref.Name},
			Key:                  ref.Key,
		}
	}
}

func convertTSAStatusFrom(src v1.TimestampAuthorityStatus, dst *TimestampAuthorityStatus) {
	if src.Signer == nil {
		return
	}
	if dst.Signer == nil {
		dst.Signer = &TimestampAuthoritySigner{}
	}
	if ref := src.Signer.CertificateChainRef; ref != nil {
		dst.Signer.CertificateChain.CertificateChainRef = &SecretKeySelector{
			LocalObjectReference: LocalObjectReference{Name: ref.Name},
			Key:                  ref.Key,
		}
	}
}
