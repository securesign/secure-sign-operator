package fips

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

func mustGenerateECKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustGenerateRSAKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustMarshalECPrivateKeyPEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustMarshalPKCS8PrivateKeyPEM(t *testing.T, key interface{}) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustMarshalPKCS1PrivateKeyPEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der := x509.MarshalPKCS1PrivateKey(key)
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustMarshalPublicKeyPEM(t *testing.T, key interface{}) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustMarshalPublicKeyDER(t *testing.T, key interface{}) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func mustCreateSelfSignedCert(t *testing.T, key interface{}, sigAlg x509.SignatureAlgorithm) []byte {
	t.Helper()

	var pubKey interface{}
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		pubKey = &k.PublicKey
	case *rsa.PrivateKey:
		pubKey = &k.PublicKey
	case ed25519.PrivateKey:
		pubKey = k.Public()
	default:
		t.Fatalf("unsupported key type: %T", key)
	}

	tmpl := &x509.Certificate{
		SerialNumber:       big.NewInt(1),
		Subject:            pkix.Name{CommonName: "test"},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(time.Hour),
		SignatureAlgorithm: sigAlg,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pubKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidatePrivateKeyPEM(t *testing.T) {
	tests := []struct {
		name    string
		pemData func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "ECDSA P-256 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalECPrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P256()))
			},
		},
		{
			name: "ECDSA P-384 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalECPrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P384()))
			},
		},
		{
			name: "ECDSA P-521 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalECPrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P521()))
			},
		},
		{
			name: "RSA 2048 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS1PrivateKeyPEM(t, mustGenerateRSAKey(t, 2048))
			},
		},
		{
			name: "RSA 4096 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS8PrivateKeyPEM(t, mustGenerateRSAKey(t, 4096))
			},
		},
		{
			name: "PKCS8 wrapped ECDSA P-256 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS8PrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P256()))
			},
		},
		{
			name: "Ed25519 passes",
			pemData: func(t *testing.T) []byte {
				_, priv, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				return mustMarshalPKCS8PrivateKeyPEM(t, priv)
			},
		},
		{
			name: "RSA 1024 fails",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS1PrivateKeyPEM(t, mustGenerateRSAKey(t, 1024))
			},
			wantErr: true,
		},
		{
			name:    "invalid PEM fails",
			pemData: func(_ *testing.T) []byte { return []byte("not a pem") },
			wantErr: true,
		},
		{
			name:    "empty input fails",
			pemData: func(_ *testing.T) []byte { return nil },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrivateKeyPEM(tt.pemData(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePrivateKeyPEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePublicKeyPEM(t *testing.T) {
	tests := []struct {
		name    string
		pemData func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "ECDSA P-256 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPublicKeyPEM(t, &mustGenerateECKey(t, elliptic.P256()).PublicKey)
			},
		},
		{
			name: "RSA 2048 passes",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPublicKeyPEM(t, &mustGenerateRSAKey(t, 2048).PublicKey)
			},
		},
		{
			name: "Ed25519 passes",
			pemData: func(t *testing.T) []byte {
				pub, _, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				return mustMarshalPublicKeyPEM(t, pub)
			},
		},
		{
			name:    "invalid PEM fails",
			pemData: func(_ *testing.T) []byte { return []byte("not a pem") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicKeyPEM(tt.pemData(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicKeyPEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePublicKeyDER(t *testing.T) {
	tests := []struct {
		name    string
		derData func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "ECDSA P-256 passes",
			derData: func(t *testing.T) []byte {
				return mustMarshalPublicKeyDER(t, &mustGenerateECKey(t, elliptic.P256()).PublicKey)
			},
		},
		{
			name: "Ed25519 passes",
			derData: func(t *testing.T) []byte {
				pub, _, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				return mustMarshalPublicKeyDER(t, pub)
			},
		},
		{
			name:    "invalid DER fails",
			derData: func(_ *testing.T) []byte { return []byte("garbage") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicKeyDER(tt.derData(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicKeyDER() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCertificateChainPEM(t *testing.T) {
	tests := []struct {
		name    string
		pemData func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "single valid cert passes",
			pemData: func(t *testing.T) []byte {
				key := mustGenerateECKey(t, elliptic.P256())
				return mustCreateSelfSignedCert(t, key, x509.ECDSAWithSHA256)
			},
		},
		{
			name: "two valid certs pass",
			pemData: func(t *testing.T) []byte {
				key1 := mustGenerateECKey(t, elliptic.P256())
				key2 := mustGenerateECKey(t, elliptic.P384())
				cert1 := mustCreateSelfSignedCert(t, key1, x509.ECDSAWithSHA256)
				cert2 := mustCreateSelfSignedCert(t, key2, x509.ECDSAWithSHA384)
				return append(cert1, cert2...)
			},
		},
		{
			name: "one bad cert in chain fails",
			pemData: func(t *testing.T) []byte {
				goodKey := mustGenerateECKey(t, elliptic.P256())
				badKey := mustGenerateRSAKey(t, 2048)
				goodCert := mustCreateSelfSignedCert(t, goodKey, x509.ECDSAWithSHA256)
				badCert := mustCreateSelfSignedCert(t, badKey, x509.SHA1WithRSA)
				return append(goodCert, badCert...)
			},
			wantErr: true,
		},
		{
			name:    "no certificates fails",
			pemData: func(_ *testing.T) []byte { return []byte("not a cert") },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCertificateChainPEM(tt.pemData(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCertificateChainPEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCryptoMaterialPEM(t *testing.T) {
	tests := []struct {
		name    string
		pemData func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "detects and validates certificate",
			pemData: func(t *testing.T) []byte {
				key := mustGenerateECKey(t, elliptic.P256())
				return mustCreateSelfSignedCert(t, key, x509.ECDSAWithSHA256)
			},
		},
		{
			name: "detects and validates public key",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPublicKeyPEM(t, &mustGenerateECKey(t, elliptic.P256()).PublicKey)
			},
		},
		{
			name: "detects and validates PKCS8 private key",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS8PrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P256()))
			},
		},
		{
			name: "detects and validates EC private key",
			pemData: func(t *testing.T) []byte {
				return mustMarshalECPrivateKeyPEM(t, mustGenerateECKey(t, elliptic.P256()))
			},
		},
		{
			name: "detects and validates RSA private key",
			pemData: func(t *testing.T) []byte {
				return mustMarshalPKCS1PrivateKeyPEM(t, mustGenerateRSAKey(t, 2048))
			},
		},
		{
			name:    "empty input fails",
			pemData: func(_ *testing.T) []byte { return nil },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCryptoMaterialPEM(tt.pemData(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCryptoMaterialPEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCryptoMaterialPEM_NoDoubleWrap(t *testing.T) {
	key := mustGenerateRSAKey(t, 1024)
	certPEM := mustCreateSelfSignedCert(t, key, x509.SHA256WithRSA)

	err := ValidateCryptoMaterialPEM(certPEM)
	if err == nil {
		t.Fatal("expected error for RSA-1024 cert, got nil")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("expected ValidationError, got", err)
	}
	inner := errors.Unwrap(err)
	var innerVE *ValidationError
	if errors.As(inner, &innerVE) {
		t.Errorf("double-wrapped ValidationError: Unwrap returned another ValidationError: %v", inner)
	}
}

func TestValidateCryptoMaterialIfPEM(t *testing.T) {
	tests := []struct {
		name    string
		data    func(t *testing.T) []byte
		wantErr bool
	}{
		{
			name: "non-PEM data returns nil",
			data: func(_ *testing.T) []byte { return []byte("not pem data") },
		},
		{
			name: "unrecognized PEM block type returns nil",
			data: func(_ *testing.T) []byte {
				return pem.EncodeToMemory(&pem.Block{Type: "CUSTOM BLOCK", Bytes: []byte("data")})
			},
		},
		{
			name: "single valid certificate passes",
			data: func(t *testing.T) []byte {
				key := mustGenerateECKey(t, elliptic.P256())
				return mustCreateSelfSignedCert(t, key, x509.ECDSAWithSHA256)
			},
		},
		{
			name: "single invalid certificate fails",
			data: func(t *testing.T) []byte {
				key := mustGenerateRSAKey(t, 2048)
				return mustCreateSelfSignedCert(t, key, x509.SHA1WithRSA)
			},
			wantErr: true,
		},
		{
			name: "cert + valid private key bundle passes",
			data: func(t *testing.T) []byte {
				key := mustGenerateECKey(t, elliptic.P256())
				cert := mustCreateSelfSignedCert(t, key, x509.ECDSAWithSHA256)
				keyPEM := mustMarshalECPrivateKeyPEM(t, key)
				return append(cert, keyPEM...)
			},
		},
		{
			name: "valid cert + invalid private key bundle fails on private key",
			data: func(t *testing.T) []byte {
				ecKey := mustGenerateECKey(t, elliptic.P256())
				cert := mustCreateSelfSignedCert(t, ecKey, x509.ECDSAWithSHA256)
				rsaKey := mustGenerateRSAKey(t, 1024)
				keyPEM := mustMarshalPKCS1PrivateKeyPEM(t, rsaKey)
				return append(cert, keyPEM...)
			},
			wantErr: true,
		},
		{
			name: "non-crypto block + valid crypto block passes",
			data: func(t *testing.T) []byte {
				custom := pem.EncodeToMemory(&pem.Block{Type: "CUSTOM", Bytes: []byte("data")})
				key := mustGenerateECKey(t, elliptic.P256())
				keyPEM := mustMarshalECPrivateKeyPEM(t, key)
				return append(custom, keyPEM...)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCryptoMaterialIfPEM(tt.data(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCryptoMaterialIfPEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
