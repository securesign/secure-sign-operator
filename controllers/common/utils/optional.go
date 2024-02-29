package utils

func OptionalBool(boolean *bool) bool {
	return boolean != nil && *boolean
}

func Pointer[T any](d T) *T {
	return &d
}
