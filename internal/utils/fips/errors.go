package fips

import "errors"

var (
	ErrNonFIPSPrivateKey  = errors.New("private key does not use a FIPS-approved algorithm")
	ErrNonFIPSPublicKey   = errors.New("public key does not use a FIPS-approved algorithm")
	ErrNonFIPSCertificate = errors.New("certificate does not use a FIPS-approved algorithm")
	ErrInvalidPEM         = errors.New("failed to decode PEM data")
	ErrInvalidDER         = errors.New("failed to parse DER data")
)

type ValidationError struct{ err error }

func (e *ValidationError) Error() string { return e.err.Error() }
func (e *ValidationError) Unwrap() error { return e.err }

func NewValidationError(err error) error {
	if err == nil {
		return nil
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return err
	}
	return &ValidationError{err: err}
}

func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
