package cryptoutil

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
)

var FIPSEnabled bool

func init() {
	FIPSEnabled = IsFIPS()
}

var (
	ErrInvalidPEM       = errors.New("fips: invalid PEM")
	ErrUnsupportedKey   = errors.New("fips: unsupported key type")
	ErrNonFIPSAlgorithm = errors.New("fips: non-FIPS-approved algorithm/curve")
	ErrKeyTooSmall      = errors.New("fips: key size below FIPS minimum")
	ErrNoPassword       = errors.New("fips: encrypted PEM but no password provided")
	ErrFailedToDecrypt  = errors.New("fips: failed to decrypt PEM")
)

// IsFIPS returns true when the host env is FIPS enabled
func IsFIPS() bool {
	const fipsPath = "/proc/sys/crypto/fips_enabled"

	data, err := os.ReadFile(fipsPath)
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(data)) == "1"
}

func ValidatePrivateKeyPEM(pemBytes []byte, password []byte) error {
	if !FIPSEnabled {
		return nil
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ErrInvalidPEM
	}

	der := block.Bytes

	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		if len(password) == 0 {
			return ErrNoPassword
		}
		decrypted, err := x509.DecryptPEMBlock(block, password) //nolint:staticcheck
		if err != nil {
			return ErrFailedToDecrypt
		}
		der = decrypted
	}

	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return validatePrivateKeyType(key)
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return validatePrivateKeyType(key)
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return validatePrivateKeyType(key)
	}
	return ErrUnsupportedKey
}

func validatePrivateKeyType(key interface{}) error {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.N.BitLen() < 2048 {
			return ErrKeyTooSmall
		}
		return nil

	case *ecdsa.PrivateKey:
		params := k.Params()
		if params == nil {
			return fmt.Errorf("%w: unknown ECDSA curve", ErrNonFIPSAlgorithm)
		}
		if params.BitSize < 256 {
			return fmt.Errorf("%w: ECDSA curve %s (%d bits)", ErrKeyTooSmall, params.Name, params.BitSize)
		}
		return nil

	default:
		return ErrUnsupportedKey
	}
}

func ValidatePublicKeyPEM(pemBytes []byte) error {
	if !FIPSEnabled {
		return nil
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ErrInvalidPEM
	}

	der := block.Bytes
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		return validatePublicKeyType(key)
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return validatePublicKeyType(key)
	}

	return ErrUnsupportedKey
}

func validatePublicKeyType(key interface{}) error {
	switch k := key.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return ErrKeyTooSmall
		}
		return nil

	case *ecdsa.PublicKey:
		params := k.Params()
		if params == nil {
			return fmt.Errorf("%w: unknown ECDSA curve", ErrNonFIPSAlgorithm)
		}
		if params.BitSize < 256 {
			return fmt.Errorf("%w: ECDSA curve %s (%d bits)", ErrKeyTooSmall, params.Name, params.BitSize)
		}
		return nil

	default:
		return ErrUnsupportedKey
	}
}

func ValidateCertificatePEM(pemBytes []byte) error {
	if !FIPSEnabled {
		return nil
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ErrInvalidPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ErrInvalidPEM
	}

	return validatePublicKeyType(cert.PublicKey)
}

//ref: https://gitlab.com/redhat-crypto/fedora-crypto-policies/-/blob/rhel9/policies/FIPS.pol
