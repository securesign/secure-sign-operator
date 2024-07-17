package errors

// IgnoreError is a helper function that calls a function returning a value and an error,
// and returns only the value, ignoring the error.
func IgnoreError[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
