package resolvePubKey

import "errors"

var (
	// ErrParseTrustBundle is returned when the Fulcio trust bundle JSON cannot be parsed.
	ErrParseTrustBundle = errors.New("failed to parse trust bundle")

	// ErrEmptyTrustBundle is returned when the trust bundle contains no certificate chains or certificates.
	ErrEmptyTrustBundle = errors.New("trust bundle contains no certificates")

	// ErrPersistStatus is returned when the status update to the API server fails.
	ErrPersistStatus = errors.New("failed to persist trust material status")

	// ErrInvalidPEM is returned when the resolved data is not valid PEM.
	ErrInvalidPEM = errors.New("resolved data is not valid PEM")
)
