package tls

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
)

const (
	ReasonResolved = "TLSResolved"
)

func Wrapper[T apis.ConditionsAwareObject](
	specTLS func(T) rhtasv1.TLS,
	statusTLS func(T) rhtasv1.TLS,
	setStatusTLS func(T, rhtasv1.TLS),
	isEnabled func(T) bool,
) func(T) *wrapper[T] {
	return func(obj T) *wrapper[T] {
		return &wrapper[T]{
			object:           obj,
			callSpecTLS:      specTLS,
			callStatusTLS:    statusTLS,
			callSetStatusTLS: setStatusTLS,
			callIsEnabled:    isEnabled,
		}
	}
}

type wrapper[T apis.ConditionsAwareObject] struct {
	object T

	callSpecTLS      func(T) rhtasv1.TLS
	callStatusTLS    func(T) rhtasv1.TLS
	callSetStatusTLS func(T, rhtasv1.TLS)
	callIsEnabled    func(T) bool
}

func (w *wrapper[T]) SpecTLS() rhtasv1.TLS {
	return w.callSpecTLS(w.object)
}

func (w *wrapper[T]) StatusTLS() rhtasv1.TLS {
	return w.callStatusTLS(w.object)
}

func (w *wrapper[T]) SetStatusTLS(tls rhtasv1.TLS) {
	w.callSetStatusTLS(w.object, tls)
}

func (w *wrapper[T]) IsEnabled() bool {
	if w.callIsEnabled == nil {
		return true
	}
	return w.callIsEnabled(w.object)
}
