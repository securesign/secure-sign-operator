package v1alpha1

import (
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/securesign/operator/api/v1beta1"
)

func (src *Rekor) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Rekor)
	dst.ObjectMeta = src.ObjectMeta
	data, err := json.Marshal(src.Spec)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &dst.Spec); err != nil {
		return err
	}
	data, err = json.Marshal(src.Status)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &dst.Status)
}

func (dst *Rekor) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.Rekor)
	dst.ObjectMeta = src.ObjectMeta
	data, err := json.Marshal(src.Spec)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &dst.Spec); err != nil {
		return err
	}
	data, err = json.Marshal(src.Status)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &dst.Status)
}
