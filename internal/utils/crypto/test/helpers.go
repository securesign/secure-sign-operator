package fipsTest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func GenerateRSAPKCS8PEM(t *testing.T, keyBits int) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal PKCS8 key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

func GenerateRSAPKCS1PEM(t *testing.T, keyBits int) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	})
}

func GenerateEncryptedRSAPEM(t *testing.T, keyBits int) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)
	password := []byte("correct-horse-battery-staple")

	block, err := x509.EncryptPEMBlock( //nolint:staticcheck
		rand.Reader,
		"RSA PRIVATE KEY",
		der,
		password,
		x509.PEMCipherAES256,
	)
	if err != nil {
		t.Fatalf("failed to encrypt PEM block: %v", err)
	}

	return pem.EncodeToMemory(block), password
}

func GenerateRSAPKIXPublicKeyPEM(t *testing.T, keyBits int) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal PKIX public key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
}

func GenerateRSAPKCS1PublicKeyPEM(t *testing.T, keyBits int) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	der := x509.MarshalPKCS1PublicKey(&key.PublicKey)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: der,
	})
}

func GenerateECCertificatePEM(passwordProtected bool, CertPassword string, curve elliptic.Curve) ([]byte, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	privateKeyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	var block *pem.Block
	if passwordProtected {
		block, err = x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", privateKeyBytes, []byte(CertPassword), x509.PEMCipher3DES) //nolint:staticcheck
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		block = &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privateKeyBytes,
		}
	}
	privateKeyPem := pem.EncodeToMemory(block)
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, nil, err
	}
	publicKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: publicKeyBytes,
		},
	)

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:         "local",
		Country:            []string{"CR"},
		Organization:       []string{"RedHat"},
		Province:           []string{"Czech Republic"},
		Locality:           []string{"Brno"},
		OrganizationalUnit: []string{"QE"},
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuer,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Issuer:                issuer,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err

	}
	root := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derBytes,
		},
	)
	return publicKeyPem, privateKeyPem, root, err
}
