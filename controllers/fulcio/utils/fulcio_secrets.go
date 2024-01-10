package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
)

type FulcioCertConfig struct {
	FulcioPrivateKey []byte
	FulcioPublicKey  []byte
	FulcioRootCert   []byte
	CertPassword     []byte
}

func SetupCerts(instance *rhtasv1alpha1.Fulcio) (*FulcioCertConfig, error) {
	fulcioConfig := &FulcioCertConfig{}
	fulcioConfig.CertPassword = common.GeneratePassword(8)
	cakey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	fulcioPrivateKey, err := createCAKey(cakey, fulcioConfig.CertPassword)
	if err != nil {
		return nil, err
	}
	fulcioConfig.FulcioPrivateKey = fulcioPrivateKey

	fulcioPublicKey, err := createCAPub(cakey)
	if err != nil {
		return nil, err
	}
	fulcioConfig.FulcioPublicKey = fulcioPublicKey

	fulcioRootCert, err := createFulcioCA(cakey, instance)
	if err != nil {
		return nil, err
	}
	fulcioConfig.FulcioRootCert = fulcioRootCert

	return fulcioConfig, nil
}

func createCAKey(key *ecdsa.PrivateKey, password []byte) ([]byte, error) {
	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, []byte(password), x509.PEMCipherAES256)
	if err != nil {
		return nil, err
	}

	var pemData bytes.Buffer
	if err := pem.Encode(&pemData, block); err != nil {
		return nil, err
	}

	return pemData.Bytes(), nil
}

func createCAPub(key *ecdsa.PrivateKey) ([]byte, error) {
	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
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

func createFulcioCA(key *ecdsa.PrivateKey, instance *rhtasv1alpha1.Fulcio) ([]byte, error) {
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:   "commonName",
		Organization: []string{instance.Spec.Certificate.OrganizationName},
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(0),
		Subject:               issuer,
		EmailAddresses:        []string{instance.Spec.Certificate.OrganizationEmail},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
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
