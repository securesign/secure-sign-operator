package serviceresolver

import (
	"errors"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveInternalUrlCallback builds the in-cluster service URL for a given CR.
// Each component registers one at init-time (e.g. internal/controller/trillian/serviceresolver).
type ResolveInternalUrlCallback[T client.Object] func(T) (string, error)

var (
	ErrNoResolver = errors.New("no resolver registered for type")
	registry      = make(map[reflect.Type]func(client.Object) (string, error))
)

// Register adds a resolver for a concrete CR type. Must be called at init-time only.
func Register[T client.Object](resolverCallback func(T) (string, error)) {
	t := reflect.TypeFor[T]()
	registry[t] = func(obj client.Object) (string, error) {
		return resolverCallback(obj.(T))
	}
}

func Resolve(obj client.Object) (string, error) {
	t := reflect.TypeOf(obj)
	resolver, ok := registry[t]
	if !ok {
		return "", ErrNoResolver
	}
	return resolver(obj)
}
