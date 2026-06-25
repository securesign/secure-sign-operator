package generateSigner

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config holds the component-specific callbacks for the generic signer secret action.
type Config[T apis.ConditionsAwareObject] struct {
	// Resolve returns true if the signer can be resolved from existing state
	// (user-provided refs, pre-existing secrets from previous operator versions).
	// When true, it must also sync the resolved refs into status.
	Resolve func(context.Context, T, client.Client) bool

	// GenerateData produces the key/cert material for a fresh installation.
	GenerateData func(context.Context, T, client.Client) (map[string][]byte, error)

	// AlignStatus copies the secret name/keys into the component-specific status fields.
	AlignStatus func(T, *corev1.Secret)

	// IsEnabled returns false for paths that don't need a secret (e.g., KMS, Tink).
	// Nil defaults to true.
	IsEnabled func(T) bool

	// MutateSecret is called before the secret is created, allowing the component
	// to add labels, annotations, or modify the secret in any way.
	// Nil is a no-op.
	MutateSecret func(T, *corev1.Secret)
}

func Wrapper[T apis.ConditionsAwareObject](cfg Config[T]) func(T) *wrapper[T] {
	return func(obj T) *wrapper[T] {
		return &wrapper[T]{
			object: obj,
			cfg:    cfg,
		}
	}
}

type wrapper[T apis.ConditionsAwareObject] struct {
	object T
	cfg    Config[T]
}

func (w *wrapper[T]) Resolve(ctx context.Context, c client.Client) bool {
	return w.cfg.Resolve(ctx, w.object, c)
}

func (w *wrapper[T]) GenerateData(ctx context.Context, c client.Client) (map[string][]byte, error) {
	return w.cfg.GenerateData(ctx, w.object, c)
}

func (w *wrapper[T]) AlignStatus(secret *corev1.Secret) {
	w.cfg.AlignStatus(w.object, secret)
}

func (w *wrapper[T]) IsEnabled() bool {
	if w.cfg.IsEnabled == nil {
		return true
	}
	return w.cfg.IsEnabled(w.object)
}

func (w *wrapper[T]) EnsureMutate() func(*corev1.Secret) error {
	return func(secret *corev1.Secret) error {
		if w.cfg.MutateSecret != nil {
			w.cfg.MutateSecret(w.object, secret)
		}
		return nil
	}
}
