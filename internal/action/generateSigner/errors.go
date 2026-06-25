package generateSigner

import "errors"

var (
	// ErrSecretNotFound is returned when a user-referenced secret does not exist.
	ErrSecretNotFound = errors.New("referenced secret not found")

	// ErrStatusSecretRead is returned when a status-referenced secret cannot
	// be read due to a transient or permission error (not NotFound).
	ErrStatusSecretRead = errors.New("could not verify status-referenced secret")

	// ErrResolveFailed is returned when the ResolveRef callback returns an error.
	ErrResolveFailed = errors.New("signer resolution failed")

	// ErrSecretGet is returned when the deterministic-named secret cannot be
	// read from the API server.
	ErrSecretGet = errors.New("failed to get signer secret")

	// ErrSecretCreate is returned when creating the signer secret fails.
	ErrSecretCreate = errors.New("failed to create signer secret")
)
