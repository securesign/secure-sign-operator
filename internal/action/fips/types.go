package fips

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CryptoRef represents a piece of cryptographic material to be validated
// for FIPS compliance.
type CryptoRef struct {
	FieldPath string
	Data      []byte
	Validate  func([]byte) error
}

// Config holds the component-specific callbacks for the generic FIPS
// validation action.
type Config[T apis.ConditionsAwareObject] struct {
	// PasswordRef extracts the password-ref selector from the instance's spec.
	// When non-nil and FIPS mode is active, Handle returns a TerminalError
	// (password-protected keys are forbidden in FIPS).
	// Nil disables the FIPS password-ref guard.
	PasswordRef func(T) *rhtasv1.SecretKeySelector

	// CryptoMaterial extracts user-provided crypto material from Kubernetes secrets
	// for FIPS validation. Returns a slice of CryptoRef values to validate.
	// Nil disables FIPS crypto validation.
	CryptoMaterial func(context.Context, T, client.Client) ([]CryptoRef, error)

	// IsEnabled returns false for paths that don't need FIPS validation
	// (e.g., KMS, Tink). Nil defaults to true.
	IsEnabled func(T) bool
}

// Wrapper creates a wrapper function from a Config, suitable for passing
// to NewAction.
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

func (w *wrapper[T]) PasswordRef() *rhtasv1.SecretKeySelector {
	if w.cfg.PasswordRef == nil {
		return nil
	}
	return w.cfg.PasswordRef(w.object)
}

func (w *wrapper[T]) CryptoMaterial(ctx context.Context, c client.Client) ([]CryptoRef, error) {
	if w.cfg.CryptoMaterial == nil {
		return nil, nil
	}
	return w.cfg.CryptoMaterial(ctx, w.object, c)
}

func (w *wrapper[T]) IsEnabled() bool {
	if w.cfg.IsEnabled == nil {
		return true
	}
	return w.cfg.IsEnabled(w.object)
}

// AppendSecretRef fetches secret data and appends a CryptoRef. If ref is nil, it's a no-op.
func AppendSecretRef(ctx context.Context, c client.Client, namespace string,
	ref *rhtasv1.SecretKeySelector, fieldPath string,
	validate func([]byte) error, refs *[]CryptoRef) error {
	if ref == nil {
		return nil
	}
	data, err := kubernetes.GetSecretData(ctx, c, namespace, ref)
	if err != nil {
		return err
	}
	*refs = append(*refs, CryptoRef{FieldPath: fieldPath, Data: data, Validate: validate})
	return nil
}
