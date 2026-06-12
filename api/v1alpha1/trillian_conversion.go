package v1alpha1

import (
	v1 "github.com/securesign/operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *Trillian) ConvertTo(dstRaw conversion.Hub) error {
	if err := marshalConvert(src, dstRaw.(runtime.Object)); err != nil {
		return err
	}
	convertTrillianStatusTo(src.Status, &dstRaw.(*v1.Trillian).Status)
	return nil
}

func (dst *Trillian) ConvertFrom(srcRaw conversion.Hub) error {
	if err := marshalConvert(srcRaw.(runtime.Object), dst); err != nil {
		return err
	}
	convertTrillianStatusFrom(srcRaw.(*v1.Trillian).Status, &dst.Status)
	return nil
}

func convertTrillianStatusTo(src TrillianStatus, dst *v1.TrillianStatus) {
	dst.Db.PvcName = src.Db.Pvc.Name
}

func convertTrillianStatusFrom(src v1.TrillianStatus, dst *TrillianStatus) {
	dst.Db.Pvc.Name = src.Db.PvcName
}
