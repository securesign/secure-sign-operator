package utils

func IsEnabled(flag *bool) bool {
return utils.pointer.BoolPtrDerefOr(flag, false)
}

func OptionalBool(boolean *bool) bool {
	return boolean != nil && *boolean
}

func Pointer[T any](d T) *T {
	return &d
}
