package utils

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

type FulcioCertConfig struct {
	PrivateKey         []byte
	PublicKey          []byte
	RootCert           []byte
	PrivateKeyPassword []byte
}

func (c FulcioCertConfig) ToMap() map[string][]byte {
	result := make(map[string][]byte)

	if len(c.PrivateKey) > 0 {
		result["private"] = c.PrivateKey
	}
	if len(c.PublicKey) > 0 {
		result["public"] = c.PublicKey
	}
	if len(c.PrivateKeyPassword) > 0 {
		result["password"] = c.PrivateKeyPassword
	}
	if len(c.RootCert) > 0 {
		result["cert"] = c.RootCert
	}

	return result
}

func CreateCAKey(key *ecdsa.PrivateKey, password []byte) ([]byte, error) {
	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, password, x509.PEMCipherAES256)
	if err != nil {
		return nil, err
	}

	var pemData bytes.Buffer
	if err := pem.Encode(&pemData, block); err != nil {
		return nil, err
	}

	return pemData.Bytes(), nil
}

func CreateCAPub(key crypto.PublicKey) ([]byte, error) {
	mPubKey, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, err
	}

	var pemPubKey bytes.Buffer
	err = pem.Encode(&pemPubKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mPubKey,
	})
	if err != nil {
		return nil, err
	}

	return pemPubKey.Bytes(), nil
}

func CreateFulcioCA(config *FulcioCertConfig, instance *rhtasv1alpha1.Fulcio) ([]byte, error) {
	var err error

	if instance.Spec.Certificate.CommonName == "" || instance.Spec.Certificate.OrganizationName == "" {
		return nil, fmt.Errorf("could not create certificate: missing OrganizationName or CommonName from config")
	}

	block, _ := pem.Decode(config.PrivateKey)
	keyBytes := block.Bytes
	if x509.IsEncryptedPEMBlock(block) {
		keyBytes, err = x509.DecryptPEMBlock(block, config.PrivateKeyPassword)
		if err != nil {
			return nil, err
		}
	}

	key, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:   instance.Spec.Certificate.CommonName,
		Organization: []string{instance.Spec.Certificate.OrganizationName},
	}

	serialNumber, err := GenerateSerialNumber()
	if err != nil {
		return nil, err
	}

	emailAddresses := make([]string, 0)

	if instance.Spec.Certificate.OrganizationEmail != "" {
		emailAddresses = append(emailAddresses, instance.Spec.Certificate.OrganizationEmail)
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               issuer,
		EmailAddresses:        emailAddresses,
		SignatureAlgorithm:    x509.ECDSAWithSHA384,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Issuer:                issuer,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}

	fulcioRoot, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		return nil, err
	}

	var pemFulcioRoot bytes.Buffer
	err = pem.Encode(&pemFulcioRoot, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fulcioRoot,
	})
	if err != nil {
		return nil, err
	}

	return pemFulcioRoot.Bytes(), nil
}

// GenerateSerialNumber creates a compliant serial number as per RFC 5280 4.1.2.2.
// Serial numbers must be positive, and can be no longer than 20 bytes.
// The serial number is generated with 159 bits, so that the first bit will always
// be 0, resulting in a positive serial number.
func GenerateSerialNumber() (*big.Int, error) {
	// Pick a random number from 0 to 2^159.
	serial, err := rand.Int(rand.Reader, (&big.Int{}).Exp(big.NewInt(2), big.NewInt(159), nil))
	if err != nil {
		return nil, errors.New("error generating serial number")
	}
	return serial, nil
}
