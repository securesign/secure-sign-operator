package cryptoutil

import (
	"context"
	"crypto/elliptic"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/securesign/operator/api/v1alpha1"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func Test_ValidateTrustedCA(t *testing.T) {
	type testCase struct {
		name        string
		fipsEnabled bool
		ca          *client.ObjectKey
		objects     []client.Object
		wantErr     bool
	}

	_, _, validCert, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
	_, _, invalidCert, _ := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())

	tests := []testCase{
		{
			name:        "FIPS disabled skips validation",
			fipsEnabled: false,
			ca:          nil,
			objects:     nil,
			wantErr:     false,
		},
		{
			name:        "FIPS enabled but no CA provided",
			fipsEnabled: true,
			ca:          nil,
			objects:     nil,
			wantErr:     false,
		},
		{
			name:        "missing ConfigMap returns error",
			fipsEnabled: true,
			ca: &client.ObjectKey{
				Name:      "missing",
				Namespace: "default",
			},
			objects: nil,
			wantErr: true,
		},
		{
			name:        "ConfigMap without PEM entries returns error",
			fipsEnabled: true,
			ca: &client.ObjectKey{
				Name:      "ca",
				Namespace: "default",
			},
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca",
						Namespace: "default",
					},
					Data: map[string]string{
						"notes": "not a certificate",
					},
				},
			},
			wantErr: true,
		},
		{
			name:        "ConfigMap with invalid PEM returns error",
			fipsEnabled: true,
			ca: &client.ObjectKey{
				Name:      "ca",
				Namespace: "default",
			},
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca",
						Namespace: "default",
					},
					Data: map[string]string{
						"ca.crt": string(invalidCert),
					},
				},
			},
			wantErr: true,
		},
		{
			name:        "ConfigMap with valid PEM passes",
			fipsEnabled: true,
			ca: &client.ObjectKey{
				Name:      "ca",
				Namespace: "default",
			},
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca",
						Namespace: "default",
					},
					Data: map[string]string{
						"ca.crt": string(validCert),
					},
				},
			},
			wantErr: false,
		},
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FIPSEnabled = tt.fipsEnabled
			cli := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			var caRef *v1alpha1.LocalObjectReference
			if tt.ca != nil {
				caRef = &v1alpha1.LocalObjectReference{Name: tt.ca.Name}
			}

			err := ValidateTrustedCA(context.Background(), cli, "default", caRef)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
