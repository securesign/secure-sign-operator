package fips

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
)

const minRSAKeySize = 2048

var fipsApprovedCurves = map[elliptic.Curve]bool{
	elliptic.P256(): true,
	elliptic.P384(): true,
	elliptic.P521(): true,
}

var fipsApprovedSignatureAlgorithms = map[x509.SignatureAlgorithm]bool{
	x509.ECDSAWithSHA256:  true,
	x509.ECDSAWithSHA384:  true,
	x509.ECDSAWithSHA512:  true,
	x509.SHA256WithRSA:    true,
	x509.SHA384WithRSA:    true,
	x509.SHA512WithRSA:    true,
	x509.SHA256WithRSAPSS: true,
	x509.SHA384WithRSAPSS: true,
	x509.SHA512WithRSAPSS: true,
	x509.PureEd25519:      true,
}

func validatePrivateKey(key crypto.PrivateKey) error {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		return validateECDSACurve(k.Curve, ErrNonFIPSPrivateKey)
	case *rsa.PrivateKey:
		return validateRSAKeySize(k.N.BitLen(), ErrNonFIPSPrivateKey)
	case ed25519.PrivateKey:
		return nil
	default:
		return fmt.Errorf("%w: %T is not FIPS-approved; use ECDSA (P-256, P-384, P-521), RSA (>= %d bits), or Ed25519",
			ErrNonFIPSPrivateKey, key, minRSAKeySize)
	}
}

func validatePublicKey(key crypto.PublicKey) error {
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		return validateECDSACurve(k.Curve, ErrNonFIPSPublicKey)
	case *rsa.PublicKey:
		return validateRSAKeySize(k.N.BitLen(), ErrNonFIPSPublicKey)
	case ed25519.PublicKey:
		return nil
	default:
		return fmt.Errorf("%w: %T is not FIPS-approved; use ECDSA (P-256, P-384, P-521), RSA (>= %d bits), or Ed25519",
			ErrNonFIPSPublicKey, key, minRSAKeySize)
	}
}

func validateSignatureAlgorithm(alg x509.SignatureAlgorithm) error {
	if fipsApprovedSignatureAlgorithms[alg] {
		return nil
	}
	return fmt.Errorf("%w: %s is not FIPS-approved; use SHA256/384/512 with RSA or ECDSA, or PureEd25519",
		ErrNonFIPSCertificate, alg)
}

func validateECDSACurve(curve elliptic.Curve, sentinel error) error {
	if fipsApprovedCurves[curve] {
		return nil
	}
	return fmt.Errorf("%w: elliptic curve %s is not FIPS-approved; use P-256, P-384, or P-521",
		sentinel, curve.Params().Name)
}

func validateRSAKeySize(bits int, sentinel error) error {
	if bits >= minRSAKeySize {
		return nil
	}
	return fmt.Errorf("%w: RSA key is %d bits, FIPS requires >= %d bits",
		sentinel, bits, minRSAKeySize)
}
