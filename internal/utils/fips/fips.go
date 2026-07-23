package fips

import (
	"crypto/fips140"
	"errors"
)

var Enabled = fips140.Enabled

var ErrPasswordRefInFIPS = errors.New("password-protected private keys are not allowed in FIPS mode: remove the password reference and provide an unencrypted key")

const (
	ClientSigningAlgorithms = "ecdsa-sha2-256-nistp256,ecdsa-sha2-384-nistp384,ecdsa-sha2-512-nistp521,rsa-sign-pkcs1-2048-sha256,rsa-sign-pkcs1-3072-sha256,rsa-sign-pkcs1-4096-sha256,ed25519,ed25519-ph"
	FIPSCondition           = "FIPSCompliant"
)

func AppendFIPSCondition(conditions []string) []string {
	if Enabled() {
		return append(conditions, FIPSCondition)
	}
	return conditions
}
