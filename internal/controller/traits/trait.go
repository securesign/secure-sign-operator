package traits

type Trait[T any] func(T) error

func Apply[T any](object T, traits ...Trait[T]) (T, error) {
	for _, trait := range traits {
		if err := trait(object); err != nil {
			return object, err
		}
	}
	return object, nil
}
