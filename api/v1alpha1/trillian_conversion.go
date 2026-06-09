package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Trillian) ConvertTo(dstRaw conversion.Hub) error {
	return marshalConvert(src, dstRaw.(runtime.Object))
}

func (dst *Trillian) ConvertFrom(srcRaw conversion.Hub) error {
	return marshalConvert(srcRaw.(runtime.Object), dst)
}
