package generateSigner

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RequireSecret verifies that the secret referenced by ref exists.
// Returns nil if found, ErrSecretNotFound if the API confirms it does not exist,
// or the raw API error for transient/permission failures.
func RequireSecret(ctx context.Context, c client.Client, namespace string, ref *rhtasv1.SecretKeySelector) error {
	if ref == nil || ref.Name == "" {
		return fmt.Errorf("%w: <nil>", ErrSecretNotFound)
	}
	found, err := kubernetes.ExistsSecret(ctx, c, namespace, ref.Name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, ref.Name)
	}
	return nil
}

// ResolveStatusSecret checks whether a status-referenced secret still exists.
// It skips the check when the ref is nil, empty, or matches the deterministic
// name (meaning the secret was created by the current operator version).
//
// Returns:
//   - (name, nil)  — secret exists, caller should reuse it
//   - ("", nil)    — ref is nil/empty, matches deterministic name, or secret not found
//   - ("", err)    — transient/permission error, wraps ErrStatusSecretRead
func ResolveStatusSecret(ctx context.Context, c client.Client, ref *rhtasv1.SecretKeySelector, namespace, deterministicName string) (*rhtasv1.SecretKeySelector, error) {
	if ref == nil || ref.Name == "" || ref.Name == deterministicName {
		return nil, nil
	}
	found, err := kubernetes.ExistsSecret(ctx, c, namespace, ref.Name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrStatusSecretRead, ref.Name, err)
	}
	if found {
		return ref, nil
	}
	return nil, nil
}
