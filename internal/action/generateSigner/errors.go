package generateSigner

import "fmt"

// ErrDataTampered indicates the secret data was modified externally.
// The stored SHA256 hash no longer matches the actual data.
func ErrDataTampered(secretName, expectedHash, actualHash string) error {
	return fmt.Errorf(
		"signer secret %q data was modified externally (expected hash %s, got %s); "+
			"operator-managed signer secrets must not be changed after creation — "+
			"restore the original secret or delete it and re-install the component",
		secretName, expectedHash, actualHash,
	)
}

// ErrConfigMismatch indicates a secret with the deterministic name already exists
// but its configuration does not match the current spec.
func ErrConfigMismatch(secretName string) error {
	return fmt.Errorf(
		"signer secret %q already exists with different configuration; "+
			"delete the existing secret or reference it in the spec to reuse it",
		secretName,
	)
}
