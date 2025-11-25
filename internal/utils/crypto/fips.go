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
	ErrNonFIPSSignature = errors.New("fips: non-FIPS-approved signature algorithm")
)

var (
	fipsApprovedCurves = map[string]struct{}{
		"P-256": {},
		"P-384": {},
		"P-521": {},
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

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ErrInvalidPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ErrInvalidPEM
	}

	if err := validateSignatureAlgorithm(cert.SignatureAlgorithm); err != nil {
		return err
	}

	return validatePublicKeyType(cert.PublicKey)
}

func ValidateTLS(client client.Client, namespace string, tls v1alpha1.TLS) error {
	if tls.CertRef != nil {
		cert, err := kubernetes.GetSecretData(client, namespace, tls.CertRef)
		if err != nil {
			return err
		}
		if err := ValidateCertificatePEM(cert); err != nil {
			return err
		}
	}

	if tls.PrivateKeyRef != nil {
		key, err := kubernetes.GetSecretData(client, namespace, tls.PrivateKeyRef)
		if err != nil {
			return err
		}
		if err := ValidatePrivateKeyPEM(key, nil); err != nil {
			return err
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

	var validated bool
	for k, v := range cm.Data {
		b := []byte(v)
		if len(b) == 0 || !bytes.Contains(b, []byte("BEGIN CERTIFICATE")) {
			continue
		}
		if err := ValidateCertificatePEM(b); err != nil {
			return fmt.Errorf("trusted CA entry %q is not FIPS-compliant: %w", k, err)
		}
		validated = true
	}
	if !validated {
		return fmt.Errorf("trusted CA %q has no PEM certificates to validate", ca.Name)
	}
	return nil
}

func validatePrivateKeyType(key interface{}) error {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.N.BitLen() < 2048 {
			return ErrKeyTooSmall
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
			return ErrKeyTooSmall
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
		return fmt.Errorf("%w: ECDSA curve %s (%d bits)", ErrKeyTooSmall, params.Name, params.BitSize)
	}
	if _, ok := fipsApprovedCurves[params.Name]; !ok {
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
