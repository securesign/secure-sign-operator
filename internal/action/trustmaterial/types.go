package trustmaterial

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver defines component-specific behavior for the generic resolve public key action.
// Each component (Rekor, Fulcio, TSA, CTlog) implements this interface with its own
// resolution logic, status field accessors, and condition gating.
type Resolver[T apis.ConditionsAwareObject] interface {
	// ComponentName returns the name used for logging and event recording.
	ComponentName() string

	// CanHandle returns true when the action should execute.
	// Typically gates on the component's readiness state (>= Initialize).
	CanHandle(ctx context.Context, instance T) bool

	// Resolve fetches trust material (public key, certificate, or cert chain)
	// from the running service or other source. Each component implements
	// its own resolution logic: HTTP call, gRPC, secret read, etc.
	// Returns PEM-encoded bytes or error.
	Resolve(ctx context.Context, cli client.Client, instance T) ([]byte, error)

	// GetTrustMaterial reads the current value of the component's status field
	// (PublicKey or CertificateChain).
	GetTrustMaterial(instance T) string

	// SetTrustMaterial writes the resolved value into the component's status field.
	SetTrustMaterial(instance T, pem string)
}
