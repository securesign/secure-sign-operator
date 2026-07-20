package trustmaterial

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestParseTrustBundle(t *testing.T) {
	t.Parallel()

	t.Run("single cert chain", func(t *testing.T) {
		t.Parallel()
		rootPEM := generateSelfSignedCert(t, "root-ca")
		json := `{"chains": [{"certificates": ["` + escapePEM(rootPEM) + `"]}]}`
		result, err := ParseTrustBundle([]byte(json))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(result), "CERTIFICATE") {
			t.Fatal("expected certificate PEM in result")
		}
	})

	t.Run("multi cert chain returns full chain", func(t *testing.T) {
		t.Parallel()
		rootKey, rootPEM := generateSelfSignedCertWithKey(t, "root-ca")
		leafPEM := generateLeafCert(t, "leaf", rootKey, rootPEM)

		json := `{"chains": [{"certificates": ["` + escapePEM(leafPEM) + `", "` + escapePEM(rootPEM) + `"]}]}`
		result, err := ParseTrustBundle([]byte(json))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count := strings.Count(string(result), "BEGIN CERTIFICATE"); count != 2 {
			t.Errorf("expected 2 certs in chain, got %d", count)
		}
	})

	t.Run("empty chains", func(t *testing.T) {
		t.Parallel()
		_, err := ParseTrustBundle([]byte(`{"chains": []}`))
		if !errors.Is(err, ErrEmptyTrustBundle) {
			t.Errorf("expected ErrEmptyTrustBundle, got: %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		_, err := ParseTrustBundle([]byte("not json"))
		if !errors.Is(err, ErrParseTrustBundle) {
			t.Errorf("expected ErrParseTrustBundle, got: %v", err)
		}
	})

	t.Run("multiple single-cert chains", func(t *testing.T) {
		t.Parallel()
		root1 := generateSelfSignedCert(t, "root-1")
		root2 := generateSelfSignedCert(t, "root-2")
		json := `{"chains": [` +
			`{"certificates": ["` + escapePEM(root1) + `"]},` +
			`{"certificates": ["` + escapePEM(root2) + `"]}` +
			`]}`
		result, err := ParseTrustBundle([]byte(json))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count := strings.Count(string(result), "BEGIN CERTIFICATE"); count != 2 {
			t.Errorf("expected 2 certs, got %d", count)
		}
	})
}

func TestValidatePEM(t *testing.T) {
	t.Parallel()

	t.Run("valid public key", func(t *testing.T) {
		t.Parallel()
		pemData := []byte(generatePublicKeyPEM(t))
		if err := ValidatePEM(pemData); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("valid certificate", func(t *testing.T) {
		t.Parallel()
		pemData := []byte(generateSelfSignedCert(t, "test"))
		if err := ValidatePEM(pemData); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("PEM-wrapped garbage is rejected", func(t *testing.T) {
		t.Parallel()
		pemData := []byte("-----BEGIN PUBLIC KEY-----\nbm90IGEgcmVhbCBrZXk=\n-----END PUBLIC KEY-----\n")
		if err := ValidatePEM(pemData); !errors.Is(err, ErrInvalidPEM) {
			t.Errorf("expected ErrInvalidPEM, got: %v", err)
		}
	})

	t.Run("not PEM data", func(t *testing.T) {
		t.Parallel()
		err := ValidatePEM([]byte("this is not PEM"))
		if !errors.Is(err, ErrInvalidPEM) {
			t.Errorf("expected ErrInvalidPEM, got: %v", err)
		}
	})

	t.Run("empty data", func(t *testing.T) {
		t.Parallel()
		err := ValidatePEM([]byte{})
		if !errors.Is(err, ErrInvalidPEM) {
			t.Errorf("expected ErrInvalidPEM, got: %v", err)
		}
	})

	t.Run("valid multi-certificate chain", func(t *testing.T) {
		t.Parallel()
		rootKey, rootPEM := generateSelfSignedCertWithKey(t, "root-ca")
		leafPEM := generateLeafCert(t, "leaf", rootKey, rootPEM)
		chain := []byte(leafPEM + rootPEM)
		if err := ValidatePEM(chain); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("corrupted block after a valid leading block is rejected", func(t *testing.T) {
		t.Parallel()
		validPEM := []byte(generateSelfSignedCert(t, "test"))
		corrupted := []byte("-----BEGIN CERTIFICATE-----\nbm90IGEgcmVhbCBjZXJ0\n-----END CERTIFICATE-----\n")
		chain := append(append([]byte{}, validPEM...), corrupted...)
		if err := ValidatePEM(chain); !errors.Is(err, ErrInvalidPEM) {
			t.Errorf("expected ErrInvalidPEM for a chain with a corrupted trailing block, got: %v", err)
		}
	})
}

func generateSelfSignedCert(t *testing.T, cn string) string {
	t.Helper()
	_, pemStr := generateSelfSignedCertWithKey(t, cn)
	return pemStr
}

func generatePublicKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func generateSelfSignedCertWithKey(t *testing.T, cn string) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating cert: %v", err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func generateLeafCert(t *testing.T, cn string, parentKey *ecdsa.PrivateKey, parentPEM string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(parentPEM))
	parentCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing parent cert: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parentCert, &leafKey.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("creating leaf cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func escapePEM(pemStr string) string {
	return strings.ReplaceAll(strings.TrimSpace(pemStr), "\n", `\n`)
}

func TestExtractSigningCert(t *testing.T) {
	t.Parallel()

	t.Run("extracts first cert from chain", func(t *testing.T) {
		t.Parallel()
		rootKey, rootPEM := generateSelfSignedCertWithKey(t, "root-ca")
		signingPEM := generateLeafCert(t, "signing", rootKey, rootPEM)

		chain := signingPEM + rootPEM
		result, err := ExtractSigningCert([]byte(chain))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resultStr := strings.TrimSpace(string(result))
		signingStr := strings.TrimSpace(signingPEM)
		if resultStr != signingStr {
			t.Errorf("expected signing cert only, got:\n%s\n\nwant:\n%s", resultStr, signingStr)
		}
	})

	t.Run("single cert returned as-is", func(t *testing.T) {
		t.Parallel()
		certPEM := generateSelfSignedCert(t, "single")
		result, err := ExtractSigningCert([]byte(certPEM))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(string(result)) != strings.TrimSpace(certPEM) {
			t.Error("expected same cert back")
		}
	})

	t.Run("invalid PEM", func(t *testing.T) {
		t.Parallel()
		_, err := ExtractSigningCert([]byte("not pem"))
		if !errors.Is(err, ErrInvalidPEM) {
			t.Errorf("expected ErrInvalidPEM, got: %v", err)
		}
	})
}

func TestResolveBaseURL(t *testing.T) {
	t.Run("returns statusUrl", func(t *testing.T) {
		got := ResolveBaseURL("fulcio-server", "test-ns", "https://fulcio.external.example.com")
		if got != "https://fulcio.external.example.com" {
			t.Errorf("expected statusUrl, got %s", got)
		}
	})

	t.Run("empty statusUrl returns empty", func(t *testing.T) {
		got := ResolveBaseURL("fulcio-server", "test-ns", "")
		if got != "" {
			t.Errorf("expected empty string, got %s", got)
		}
	})
}
