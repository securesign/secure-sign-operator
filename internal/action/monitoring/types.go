package monitoring

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
)

// Config defines component-specific behavior that depends on the CRD instance.
// Static naming and labeling are passed as constructor parameters to [NewAction].
type Config[T apis.ConditionsAwareObject] interface {
	// IsEnabled reports whether ServiceMonitor creation is enabled.
	IsEnabled(instance T) bool

	// TLS returns the TLS configuration for HTTPS endpoints.
	// Return zero value with nil CertRef for plain HTTP endpoints.
	TLS(instance T) rhtasv1.TLS
}
