package cryptoutil

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	FIPSEnabled         bool
	ErrInvalidPEM       = errors.New("fips: invalid PEM")
	ErrUnsupportedKey   = errors.New("fips: unsupported key type")
	ErrNonFIPSAlgorithm = errors.New("fips: non-FIPS-approved algorithm/curve")
	ErrKeyTooSmall      = errors.New("fips: key size below FIPS minimum")
	ErrNoPassword       = errors.New("fips: encrypted PEM but no password provided")
	ErrFailedToDecrypt  = errors.New("fips: failed to decrypt PEM")
	ErrNonFIPSSignature = errors.New("fips: non-FIPS-approved signature algorithm")

	fipsApprovedCurves = map[string]elliptic.Curve{
		"P-256": elliptic.P256(),
		"P-384": elliptic.P384(),
		"P-521": elliptic.P521(),
	}

	fipsApprovedSignatureAlgos = map[x509.SignatureAlgorithm]struct{}{
		x509.SHA256WithRSA:    {},
		x509.SHA384WithRSA:    {},
		x509.SHA512WithRSA:    {},
		x509.ECDSAWithSHA256:  {},
		x509.ECDSAWithSHA384:  {},
		x509.ECDSAWithSHA512:  {},
		x509.SHA256WithRSAPSS: {},
		x509.SHA384WithRSAPSS: {},
		x509.SHA512WithRSAPSS: {},
	}
)

func init() {
	FIPSEnabled = IsFIPS()
}

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
			return fmt.Errorf("%w: %v", ErrFailedToDecrypt, err)
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

func ValidateCertificatePEM(pemBytes []byte) error {
	if !FIPSEnabled {
		return nil
	}

	if len(bytes.TrimSpace(pemBytes)) == 0 {
		return ErrInvalidPEM
	}

	for {
		block, rest := pem.Decode(pemBytes)
		if block == nil {
			if len(bytes.TrimSpace(pemBytes)) == 0 {
				return nil
			}
			return fmt.Errorf("%w: trailing data after PEM block", ErrInvalidPEM)
		}

		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("%w: unexpected PEM block %q", ErrInvalidPEM, block.Type)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("%w: failed to parse certificate: %v", ErrInvalidPEM, err)
		}

		if err := validateSignatureAlgorithm(cert.SignatureAlgorithm); err != nil {
			return fmt.Errorf("certificate signature: %w", err)
		}

		if err := validatePublicKeyType(cert.PublicKey); err != nil {
			return fmt.Errorf("certificate public key: %w", err)
		}

		pemBytes = rest
	}
}

func ValidatePublicKeyDER(der []byte) error {
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
	if len(pemBytes) == 0 {
		return fmt.Errorf("failed to encode public key")
	}
	return ValidatePublicKeyPEM(pemBytes)
}

func ValidateTLS(client client.Client, namespace string, tls v1alpha1.TLS) error {
	if tls.CertRef != nil {
		cert, err := kubernetes.GetSecretData(client, namespace, tls.CertRef)
		if err != nil {
			return fmt.Errorf("failed to get certificate: %w", err)
		}
		if err := ValidateCertificatePEM(cert); err != nil {
			return fmt.Errorf("certificate validation failed: %w", err)
		}
	}

	if tls.PrivateKeyRef != nil {
		key, err := kubernetes.GetSecretData(client, namespace, tls.PrivateKeyRef)
		if err != nil {
			return fmt.Errorf("failed to get private key: %w", err)
		}
		if err := ValidatePrivateKeyPEM(key, nil); err != nil {
			return fmt.Errorf("private key validation failed: %w", err)
		}
	}

	return nil
}

func ValidateTrustedCA(ctx context.Context, c client.Client, namespace string, ca *v1alpha1.LocalObjectReference) error {
	if !FIPSEnabled || ca == nil {
		return nil
	}

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ca.Name}, cm); err != nil {
		return fmt.Errorf("could not load trusted CA %q: %w", ca.Name, err)
	}

	for k, v := range cm.Data {
		rest := []byte(v)

		for {
			block, remaining := pem.Decode(rest)
			if block == nil {
				if len(rest) == 0 {
					break
				}
				return fmt.Errorf("%w: trusted CA entry %q contains invalid PEM", ErrInvalidPEM, k)
			}

			if block.Type != "CERTIFICATE" {
				return fmt.Errorf("%w: trusted CA entry %q has unexpected PEM block %q", ErrInvalidPEM, k, block.Type)
			}

			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return fmt.Errorf("%w: trusted CA entry %q failed to parse certificate: %v", ErrInvalidPEM, k, err)
			}

			if err := validateSignatureAlgorithm(cert.SignatureAlgorithm); err != nil {
				return fmt.Errorf("trusted CA entry %q certificate signature: %w", k, err)
			}
			if err := validatePublicKeyType(cert.PublicKey); err != nil {
				return fmt.Errorf("trusted CA entry %q certificate public key: %w", k, err)
			}

			rest = remaining
		}
	}

	return nil
}

func validatePrivateKeyType(key interface{}) error {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.N.BitLen() < 2048 {
			return fmt.Errorf("%w: RSA key size %d bits (minimum 2048)", ErrKeyTooSmall, k.N.BitLen())
		}
		return nil

	case *ecdsa.PrivateKey:
		return validateECDSACurve(k.Params())

	default:
		return ErrUnsupportedKey
	}
}

func validatePublicKeyType(key interface{}) error {
	switch k := key.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return fmt.Errorf("%w: RSA key size %d bits (minimum 2048)", ErrKeyTooSmall, k.N.BitLen())
		}
		return nil

	case *ecdsa.PublicKey:
		return validateECDSACurve(k.Params())

	default:
		return ErrUnsupportedKey
	}
}

func validateECDSACurve(params *elliptic.CurveParams) error {
	if params == nil {
		return fmt.Errorf("%w: unknown ECDSA curve", ErrNonFIPSAlgorithm)
	}

	if params.BitSize < 256 {
		return fmt.Errorf("%w: ECDSA curve %s (%d bits, minimum 256)", ErrKeyTooSmall, params.Name, params.BitSize)
	}

	_, ok := fipsApprovedCurves[params.Name]
	if !ok {
		return fmt.Errorf("%w: ECDSA curve %s", ErrNonFIPSAlgorithm, params.Name)
	}
	return nil
}

func validateSignatureAlgorithm(algo x509.SignatureAlgorithm) error {
	if _, ok := fipsApprovedSignatureAlgos[algo]; ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNonFIPSSignature, algo.String())
}

//ref: https://gitlab.com/redhat-crypto/fedora-crypto-policies/-/blob/rhel9/policies/FIPS.pol
