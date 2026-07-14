package fips

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const (
	pemTypeCertificate = "CERTIFICATE"
	pemTypePublicKey   = "PUBLIC KEY"
	pemTypePrivateKey  = "PRIVATE KEY"
	pemTypeRSAKey      = "RSA PRIVATE KEY"
	pemTypeECKey       = "EC PRIVATE KEY"
)

// ValidatePrivateKeyPEM parses a PEM-encoded private key and returns an error
// if the key algorithm or parameters are not FIPS-approved.
// Accepts PKCS#8, PKCS#1 (RSA), and SEC 1 (EC) PEM blocks.
func ValidatePrivateKeyPEM(pemData []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("%w: no PEM block found in private key data", ErrInvalidPEM)
	}

	key, err := parsePrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPEM, err)
	}
	return validatePrivateKey(key)
}

// ValidatePublicKeyPEM parses a PEM-encoded public key (PKIX/SubjectPublicKeyInfo)
// and returns an error if the key algorithm or parameters are not FIPS-approved.
func ValidatePublicKeyPEM(pemData []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("%w: no PEM block found in public key data", ErrInvalidPEM)
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPEM, err)
	}
	return validatePublicKey(key)
}

// ValidatePublicKeyDER parses a DER-encoded public key and validates FIPS compliance.
func ValidatePublicKeyDER(derData []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	key, err := x509.ParsePKIXPublicKey(derData)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDER, err)
	}
	return validatePublicKey(key)
}

// ValidatePublicKeyPEMOrDER tries PEM decoding first; if the data is not PEM-encoded,
// falls back to DER parsing. Returns an error if the key is not FIPS-approved.
func ValidatePublicKeyPEMOrDER(data []byte) error {
	block, _ := pem.Decode(data)
	if block != nil {
		if block.Type != pemTypePublicKey {
			return NewValidationError(fmt.Errorf("%w: expected %q PEM block, got %q", ErrInvalidPEM, pemTypePublicKey, block.Type))
		}
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return NewValidationError(fmt.Errorf("%w: %w", ErrInvalidPEM, err))
		}
		return NewValidationError(validatePublicKey(key))
	}
	return ValidatePublicKeyDER(data)
}

// ValidateCertificateChainPEM parses a PEM bundle containing one or more certificates
// and validates that every certificate in the chain uses FIPS-approved algorithms.
func ValidateCertificateChainPEM(pemData []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	rest := pemData
	idx := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != pemTypeCertificate {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("%w: certificate at index %d: %w", ErrInvalidPEM, idx, err)
		}
		if err := validatePublicKey(cert.PublicKey); err != nil {
			return fmt.Errorf("certificate at index %d: %w", idx, err)
		}
		if err := validateSignatureAlgorithm(cert.SignatureAlgorithm); err != nil {
			return fmt.Errorf("certificate at index %d: %w", idx, err)
		}
		idx++
	}

	if idx == 0 {
		return fmt.Errorf("%w: no certificates found in PEM data", ErrInvalidPEM)
	}
	return nil
}

// ValidateCryptoMaterialPEM auto-detects the PEM block type (CERTIFICATE, PUBLIC KEY,
// or private key variants) and validates FIPS compliance for every block in the bundle.
func ValidateCryptoMaterialPEM(pemData []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	rest := pemData
	count := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if err := validateSingleBlock(block); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("%w: no PEM block found", ErrInvalidPEM)
	}
	return nil
}

// ValidateCryptoMaterialIfPEM validates FIPS compliance only when data is
// PEM-encoded crypto material. Returns nil for non-PEM data (e.g. protobuf
// config, passwords) and for PEM blocks with unrecognized types, avoiding
// false rejections of non-crypto PEM data. Unlike ValidateCryptoMaterialPEM,
// this function iterates all PEM blocks in the data, validating each
// recognized crypto block individually.
func ValidateCryptoMaterialIfPEM(data []byte) (retErr error) {
	defer func() { retErr = NewValidationError(retErr) }()

	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if !isCryptoPEMBlockType(block.Type) {
			continue
		}
		if err := validateSingleBlock(block); err != nil {
			return err
		}
	}
	return nil
}

func validateSingleBlock(block *pem.Block) error {
	switch block.Type {
	case pemTypeCertificate:
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPEM, err)
		}
		if err := validatePublicKey(cert.PublicKey); err != nil {
			return err
		}
		return validateSignatureAlgorithm(cert.SignatureAlgorithm)
	case pemTypePublicKey:
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPEM, err)
		}
		return validatePublicKey(key)
	case pemTypePrivateKey, pemTypeRSAKey, pemTypeECKey:
		key, err := parsePrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPEM, err)
		}
		return validatePrivateKey(key)
	default:
		return fmt.Errorf("%w: unsupported PEM block type %q", ErrInvalidPEM, block.Type)
	}
}

func isCryptoPEMBlockType(blockType string) bool {
	switch blockType {
	case pemTypeCertificate, pemTypePublicKey, pemTypePrivateKey, pemTypeRSAKey, pemTypeECKey:
		return true
	default:
		return false
	}
}

// parsePrivateKey tries PKCS#8, PKCS#1, and SEC 1 formats in sequence.
func parsePrivateKey(der []byte) (interface{}, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("failed to parse private key in any supported format (PKCS#8, PKCS#1, SEC 1)")
}
