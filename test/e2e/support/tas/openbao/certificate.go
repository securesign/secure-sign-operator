package openbao

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// oidExtKeyUsageTimeStamping is the id-kp-timeStamping OID (1.3.6.1.5.5.7.3.8).
// RFC 3161 requires a timestamping signer's certificate to carry this as a
// critical extended key usage extension.
var oidExtKeyUsageTimeStamping = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 8}

// CertChainSecretName returns the name of the Secret CreateKMSCertificate /
// CreateKMSTimestampCertificate store the resulting cert chain under, for a
// given transit key name.
func CertChainSecretName(keyName string) string {
	return keyName + "-cert-chain"
}

func newSerialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func storeCertChainSecret(ctx context.Context, cli client.Client, namespace, keyName string, chainPEM []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CertChainSecretName(keyName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"cert": chainPEM,
		},
	}
	if err := cli.Create(ctx, secret); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating secret %s: %w", secret.Name, err)
	}
	return nil
}

// CreateKMSCertificate builds a self-signed CA certificate whose public key is the
// given OpenBao transit key (signed by OpenBao itself via Signer), and stores it as
// a Secret named CertChainSecretName(keyName) under the "cert" data key, ready to be
// referenced by a component's signer.certificateChain.certificateChainRef.
//
// Fulcio's kmsca accepts a chain of a single certificate (fulcio/pkg/ca/common.go
// only requires "at least one certificate"), so this single self-signed shape is
// sufficient for Fulcio. It is NOT sufficient for TSA — see CreateKMSTimestampCertificate.
func CreateKMSCertificate(ctx context.Context, cli client.Client, namespace, keyName string) error {
	signer, err := NewSigner(ctx, namespace, keyName)
	if err != nil {
		return fmt.Errorf("creating signer for transit key %s: %w", keyName, err)
	}

	serialNumber, err := newSerialNumber()
	if err != nil {
		return fmt.Errorf("generating serial number: %w", err)
	}

	notBefore := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: keyName, Organization: []string{"RHTAS"}},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(10 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	if err != nil {
		return fmt.Errorf("creating certificate for transit key %s: %w", keyName, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	return storeCertChainSecret(ctx, cli, namespace, keyName, certPEM)
}

// CreateKMSTimestampCertificate builds a two-certificate chain (root + leaf) for a
// timestamping signer: a throwaway local root CA (not backed by any KMS key) issues
// a leaf certificate whose public key is the given OpenBao transit key. TSA signs
// timestamp responses directly with the transit key at runtime, so only the leaf's
// public key needs to match it — the root's private key is used purely to make this
// a real chain and is discarded once the certs are created.
//
// A bare self-signed cert (as CreateKMSCertificate produces for Fulcio) is NOT
// sufficient here: timestamp-authority panics at startup with "certificate chain
// must contain at least two certificates" if given only one. The leaf also carries
// a critical id-kp-timeStamping EKU, which RFC 3161 requires for a TSA responder cert.
func CreateKMSTimestampCertificate(ctx context.Context, cli client.Client, namespace, keyName string) error {
	pemStr, err := TransitPublicKeyPEM(ctx, namespace, keyName)
	if err != nil {
		return fmt.Errorf("fetching public key for transit key %s: %w", keyName, err)
	}
	leafPub, err := parseECDSAPublicKey([]byte(pemStr))
	if err != nil {
		return fmt.Errorf("parsing public key for transit key %s: %w", keyName, err)
	}

	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generating throwaway root key: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour)

	rootSerial, err := newSerialNumber()
	if err != nil {
		return fmt.Errorf("generating root serial number: %w", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          rootSerial,
		Subject:               pkix.Name{CommonName: keyName + "-root", Organization: []string{"RHTAS"}},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return fmt.Errorf("creating root certificate: %w", err)
	}
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})

	leafSerial, err := newSerialNumber()
	if err != nil {
		return fmt.Errorf("generating leaf serial number: %w", err)
	}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{oidExtKeyUsageTimeStamping})
	if err != nil {
		return fmt.Errorf("encoding timestamping EKU: %w", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber:       leafSerial,
		Subject:            pkix.Name{CommonName: keyName, Organization: []string{"RHTAS"}},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		ExtraExtensions: []pkix.Extension{{
			Id:       asn1.ObjectIdentifier{2, 5, 29, 37}, // id-ce-extKeyUsage
			Critical: true,
			Value:    ekuValue,
		}},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootTemplate, leafPub, rootKey)
	if err != nil {
		return fmt.Errorf("creating leaf certificate: %w", err)
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	chainPEM := append(append([]byte{}, leafPEM...), rootPEM...)

	return storeCertChainSecret(ctx, cli, namespace, keyName, chainPEM)
}
