package v1beta1

// Hub marks v1beta1.Fulcio as the hub (storage) version for conversion.
// controller-runtime uses this marker to route conversion through this type.
func (*Fulcio) Hub() {}
