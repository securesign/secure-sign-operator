package generateSigner

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config holds the component-specific callbacks for the generic signer secret action.
type Config[T apis.ConditionsAwareObject] struct {
	// ResolveRef returns a reference to a pre-existing secret that the component
	// should use (user-provided refs, pre-existing secrets from previous operator
	// versions). Returns nil when no pre-existing secret applies and the action
	// should generate new keys.
	// Returns (nil, err) when user-provided refs are set but the referenced secret
	// doesn't exist or can't be read — the action treats this as a retriable error.
	// For cert-based components, return the cert/chain ref (not the private key ref)
	// so TUF autodiscovery labels are applied to the correct secret.
	ResolveRef func(context.Context, T, client.Client) (*rhtasv1.SecretKeySelector, error)

	// GenerateData produces the key/cert material for a fresh installation.
	GenerateData func(context.Context, T, client.Client) (map[string][]byte, error)

	// AlignStatus writes secret reference fields into component-specific status.
	// Called on every path (resolved, existing, created) with the secret ref to use.
	// Components check instance.Spec to decide whether to copy user-provided refs
	// or construct refs with well-known keys from the secret name.
	AlignStatus func(T, rhtasv1.SecretKeySelector)

	// IsEnabled returns false for paths that don't need a secret (e.g., KMS, Tink).
	// Nil defaults to true.
	IsEnabled func(T) bool

	// MutateSecret is called before the secret is created, allowing the component
	// to add labels, annotations, or modify the secret in any way.
	// Nil is a no-op.
	MutateSecret func(T, *corev1.Secret)

	// PasswordRef extracts the password-ref selector from the instance's spec.
	// When non-nil and FIPS mode is active, Handle returns a TerminalError
	// if the selector is non-nil (password-protected keys are forbidden in FIPS).
	// Nil disables the FIPS password-ref guard.
	PasswordRef func(T) *rhtasv1.SecretKeySelector
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

func (w *wrapper[T]) ResolveRef(ctx context.Context, c client.Client) (*rhtasv1.SecretKeySelector, error) {
	return w.cfg.ResolveRef(ctx, w.object, c)
}

func (w *wrapper[T]) PasswordRef() *rhtasv1.SecretKeySelector {
	if w.cfg.PasswordRef == nil {
		return nil
	}
	return w.cfg.PasswordRef(w.object)
}

func (w *wrapper[T]) GenerateData(ctx context.Context, c client.Client) (map[string][]byte, error) {
	return w.cfg.GenerateData(ctx, w.object, c)
}

func (w *wrapper[T]) AlignStatus(ref rhtasv1.SecretKeySelector) {
	w.cfg.AlignStatus(w.object, ref)
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
