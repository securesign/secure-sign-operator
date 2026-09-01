package openbao

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"strings"

	k8sSupport "github.com/securesign/operator/test/e2e/support/kubernetes"
)

// Signer implements crypto.Signer backed by an OpenBao transit key. It is driven
// entirely through ExecInPod + the "bao" CLI already installed in the OpenBao pod —
// no Vault/OpenBao SDK dependency and no port-forwarding from the test process.
type Signer struct {
	ctx       context.Context
	namespace string
	keyName   string
	pub       *ecdsa.PublicKey
}

// NewSigner returns a crypto.Signer for the given transit key. It fetches and caches
// the key's public key immediately, so construction fails fast if the key is missing.
func NewSigner(ctx context.Context, namespace, keyName string) (*Signer, error) {
	pemStr, err := TransitPublicKeyPEM(ctx, namespace, keyName)
	if err != nil {
		return nil, fmt.Errorf("fetching public key for transit key %s: %w", keyName, err)
	}
	pub, err := parseECDSAPublicKey([]byte(pemStr))
	if err != nil {
		return nil, fmt.Errorf("parsing public key for transit key %s: %w", keyName, err)
	}
	return &Signer{ctx: ctx, namespace: namespace, keyName: keyName, pub: pub}, nil
}

func (s *Signer) Public() crypto.PublicKey {
	return s.pub
}

// Sign signs an already-hashed digest via OpenBao's transit/sign endpoint, using
// "prehashed" input so OpenBao signs the digest x509.CreateCertificate computed
// rather than re-hashing it. The returned bytes are the raw ASN.1 DER ECDSA
// signature, exactly what x509.CreateCertificate expects back from a Signer.
func (s *Signer) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	input := base64.StdEncoding.EncodeToString(digest)

	script := fmt.Sprintf(
		`export VAULT_ADDR=http://127.0.0.1:%d VAULT_TOKEN=%s; bao write -format=json transit/sign/%s input=%s prehashed=true hash_algorithm=sha2-256`,
		port, RootToken, s.keyName, input)

	out, err := k8sSupport.ExecInPodWithOutput(s.ctx, PodName, containerName, s.namespace, "/bin/sh", "-c", script)
	if err != nil {
		return nil, fmt.Errorf("signing with transit key %s: %w", s.keyName, err)
	}

	var resp struct {
		Data struct {
			Signature string `json:"signature"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parsing sign response for transit key %s: %w (raw: %s)", s.keyName, err, out)
	}

	// Signature format: "vault:v<version>:<base64 ASN.1 DER signature>".
	parts := strings.SplitN(resp.Data.Signature, ":", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected signature format from transit key %s: %q", s.keyName, resp.Data.Signature)
	}
	return base64.StdEncoding.DecodeString(parts[2])
}

// parseECDSAPublicKey decodes a PEM-encoded PKIX public key and asserts it's ECDSA.
func parseECDSAPublicKey(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to PEM-decode public key: %s", pemBytes)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("expected ECDSA public key, got %T", pub)
	}
	return ecdsaPub, nil
}
