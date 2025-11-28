package cryptoutil

import (
	"crypto/elliptic"
	"encoding/pem"
	"errors"
	"testing"

	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
)

func Test_ValidatePrivateKeyPEM(t *testing.T) {
	type testCase struct {
		name        string
		fipsEnabled bool
		build       func(t *testing.T) (pemBytes, password []byte)
		wantErr     error
	}

	tests := []testCase{
		{
			name:        "FIPS disabled returns nil",
			fipsEnabled: false,
			build: func(t *testing.T) ([]byte, []byte) {
				return []byte{}, nil
			},
			wantErr: nil,
		},
		{
			name:        "invalid PEM with FIPS enabled returns ErrInvalidPEM",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return []byte{}, nil
			},
			wantErr: ErrInvalidPEM,
		},
		{
			name:        "valid PKCS8 RSA key passes",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return fipsTest.GenerateRSAPKCS8PEM(t, 2048), nil
			},
			wantErr: nil,
		},
		{
			name:        "invalid PKCS8 RSA key fails",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return fipsTest.GenerateRSAPKCS8PEM(t, 1024), nil
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "valid PKCS1 RSA key passes",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return fipsTest.GenerateRSAPKCS1PEM(t, 2048), nil
			},
			wantErr: nil,
		},
		{
			name:        "invalid PKCS1 RSA key fails",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return fipsTest.GenerateRSAPKCS1PEM(t, 1024), nil
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "valid EC key passes",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				_, priv, _, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
				return priv, nil
			},
			wantErr: nil,
		},
		{
			name:        "invalid EC key fails",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				_, priv, _, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
				return priv, nil
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "encrypted PEM without password returns error",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				pemBytes, _ := fipsTest.GenerateEncryptedRSAPEM(t, 2048)
				return pemBytes, nil
			},
			wantErr: ErrNoPassword,
		},
		{
			name:        "encrypted PEM with wrong password returns decrypt error",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				pemBytes, _ := fipsTest.GenerateEncryptedRSAPEM(t, 2048)
				return pemBytes, []byte("wrong-password")
			},
			wantErr: ErrFailedToDecrypt,
		},
		{
			name:        "encrypted PEM with correct password passes",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				return fipsTest.GenerateEncryptedRSAPEM(t, 2048)
			},
			wantErr: nil,
		},
		{
			name:        "unsupported key type returns ErrUnsupportedKey",
			fipsEnabled: true,
			build: func(t *testing.T) ([]byte, []byte) {
				unsupportedKeyPEM := pem.EncodeToMemory(&pem.Block{
					Type:  "PRIVATE KEY",
					Bytes: []byte("this is not a valid private key DER"),
				})
				return unsupportedKeyPEM, nil
			},
			wantErr: ErrUnsupportedKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FIPSEnabled = tt.fipsEnabled

			pemBytes, password := tt.build(t)
			err := ValidatePrivateKeyPEM(pemBytes, password)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func Test_ValidatePublicKeyPEM(t *testing.T) {
	type testCase struct {
		name        string
		fipsEnabled bool
		build       func(t *testing.T) []byte
		wantErr     error
	}

	tests := []testCase{
		{
			name:        "FIPS disabled returns nil",
			fipsEnabled: false,
			build: func(t *testing.T) []byte {
				return []byte{}
			},
			wantErr: nil,
		},
		{
			name:        "invalid PEM with FIPS enabled returns ErrInvalidPEM",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return []byte{}
			},
			wantErr: ErrInvalidPEM,
		},
		{
			name:        "valid PKIX RSA key passes",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return fipsTest.GenerateRSAPKIXPublicKeyPEM(t, 2048)
			},
			wantErr: nil,
		},
		{
			name:        "invalid PKIX RSA key fails",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return fipsTest.GenerateRSAPKIXPublicKeyPEM(t, 1024)
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "valid PKCS1 RSA key passes",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return fipsTest.GenerateRSAPKCS1PublicKeyPEM(t, 2048)
			},
			wantErr: nil,
		},
		{
			name:        "invalid PKCS1 RSA key fails",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return fipsTest.GenerateRSAPKCS1PublicKeyPEM(t, 1024)
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "valid EC key passes",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				pub, _, _, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
				return pub
			},
			wantErr: nil,
		},
		{
			name:        "invalid EC key fails",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				pub, _, _, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
				return pub
			},
			wantErr: ErrKeyTooSmall,
		},
		{
			name:        "unsupported key type returns ErrUnsupportedKey",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				unsupportedKeyPEM := pem.EncodeToMemory(&pem.Block{
					Type:  "PUBLIC KEY",
					Bytes: []byte("this is not a valid public key DER"),
				})
				return unsupportedKeyPEM
			},
			wantErr: ErrUnsupportedKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FIPSEnabled = tt.fipsEnabled

			pemBytes := tt.build(t)
			err := ValidatePublicKeyPEM(pemBytes)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func Test_ValidateCertificatePEM(t *testing.T) {
	type testCase struct {
		name        string
		fipsEnabled bool
		build       func(t *testing.T) []byte
		wantErr     error
	}

	tests := []testCase{
		{
			name:        "FIPS disabled returns nil",
			fipsEnabled: false,
			build: func(t *testing.T) []byte {
				return []byte{}
			},
			wantErr: nil,
		},
		{
			name:        "invalid PEM with FIPS enabled returns ErrInvalidPEM",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				return []byte{}
			},
			wantErr: ErrInvalidPEM,
		},
		{
			name:        "valid EC certificate passes",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				_, _, cert, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
				return cert
			},
			wantErr: nil,
		},
		{
			name:        "invalid EC certificate fails",
			fipsEnabled: true,
			build: func(t *testing.T) []byte {
				_, _, cert, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
				return cert
			},
			wantErr: ErrKeyTooSmall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FIPSEnabled = tt.fipsEnabled

			pemBytes := tt.build(t)
			err := ValidateCertificatePEM(pemBytes)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}
