package utils

import (
	"k8s.io/utils/pointer"
)

func IsEnabled(flag *bool) bool {
	return pointer.BoolPtrDerefOr(flag, false)
}

func OptionalBool(boolean *bool) bool {
	return boolean != nil && *boolean
}

func Pointer[T any](d T) *T {
	return &d
}
