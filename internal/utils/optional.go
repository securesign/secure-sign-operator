package utils

import (
	"k8s.io/utils/ptr"
)

func IsEnabled(flag *bool) bool {
	return ptr.Deref(flag, false)
}

func OptionalBool(boolean *bool) bool {
	return boolean != nil && *boolean
}

func Pointer[T any](d T) *T {
	return &d
}
