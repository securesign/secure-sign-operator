package v1

func setDefault[T comparable](dst *T, val T) {
	var zero T
	if *dst == zero {
		*dst = val
	}
}

func setDefaultSlice[T any](dst *[]T, val []T) {
	if len(*dst) == 0 {
		*dst = val
	}
}
