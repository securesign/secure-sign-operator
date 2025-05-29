package tsaUtils

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TsaCertChainConfig struct {
	RootPrivateKey                  []byte
	RootPrivateKeyPassword          []byte
	IntermediatePrivateKeys         [][]byte
	IntermediatePrivateKeyPasswords [][]byte
	LeafPrivateKey                  []byte
	LeafPrivateKeyPassword          []byte
	CertificateChain                []byte
}

type Issuer struct {
	subject        pkix.Name
	emailAddresses []string
}

func (c TsaCertChainConfig) ToMap() map[string][]byte {
	result := make(map[string][]byte)

	if len(c.RootPrivateKey) > 0 {
		result["rootPrivateKey"] = c.RootPrivateKey
	}
	if len(c.RootPrivateKeyPassword) > 0 {
		result["rootPrivateKeyPassword"] = c.RootPrivateKeyPassword
	}
	for i, interPrivateKey := range c.IntermediatePrivateKeys {
		if len(interPrivateKey) > 0 {
			result[fmt.Sprintf("interPrivateKey-%d", i)] = interPrivateKey
		}
	}
	for i, interPrivateKeyPassword := range c.IntermediatePrivateKeyPasswords {
		if len(interPrivateKeyPassword) > 0 {
			result[fmt.Sprintf("interPrivateKeyPassword-%d", i)] = interPrivateKeyPassword
		}
	}
	if len(c.LeafPrivateKey) > 0 {
		result["leafPrivateKey"] = c.LeafPrivateKey
	}
	if len(c.LeafPrivateKeyPassword) > 0 {
		result["leafPrivateKeyPassword"] = c.LeafPrivateKeyPassword
	}
	if len(c.CertificateChain) > 0 {
		result["certificateChain"] = c.CertificateChain
	}

	return result
}

func CreatePrivateKey(key *ecdsa.PrivateKey, password []byte) ([]byte, error) {
	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, password, x509.PEMCipherAES256) //nolint:staticcheck
	if err != nil {
		return nil, err
	}

	var pemData bytes.Buffer
	if err := pem.Encode(&pemData, block); err != nil {
		return nil, err
	}

	return pemData.Bytes(), nil
}

func CreateTSACertChain(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority, deploymentName string, client client.Client, config *TsaCertChainConfig) ([]byte, error) {
	var err error

	rootIssuer, err := CreateCAIssuer(instance, instance.Spec.Signer.CertificateChain.RootCA, ctx, deploymentName, client)
	if err != nil {
		return nil, err
	}

	rootCertTemplate, err := generateCertTemplate(*rootIssuer, true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, nil, nil)
	if err != nil {
		return nil, err
	}

	rootPrivKey, err := parsePrivateKey(config.RootPrivateKey, config.RootPrivateKeyPassword)
	if err != nil {
		return nil, err
	}

	rootCert, err := createCertificate(rootCertTemplate, rootCertTemplate, rootPrivKey, rootPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create root CA certificate: %v", err)
	}

	rootPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rootCert,
	})

	oidExtendedKeyUsage := asn1.ObjectIdentifier{2, 5, 29, 37}
	oidTimeStamping := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 8}
	ekuValues, err := asn1.Marshal([]asn1.ObjectIdentifier{oidTimeStamping})
	if err != nil {
		return nil, fmt.Errorf("Failed to encode EKU values: %s", err)
	}
	ekuExtension := pkix.Extension{
		Id:       oidExtendedKeyUsage,
		Critical: true,
		Value:    ekuValues,
	}

	var intermediateCerts []byte
	for index, intermediateKey := range instance.Spec.Signer.CertificateChain.IntermediateCA {
		intermediateIssuer, err := CreateCAIssuer(instance, intermediateKey, ctx, deploymentName, client)
		if err != nil {
			return nil, err
		}

		intermediateCertTemplate, err := generateCertTemplate(*intermediateIssuer, true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}, []pkix.Extension{ekuExtension})
		if err != nil {
			return nil, err
		}

		intermediatePrivateKey, err := parsePrivateKey(config.IntermediatePrivateKeys[index], config.IntermediatePrivateKeyPasswords[index])
		if err != nil {
			return nil, err
		}

		intermediateCert, err := createCertificate(intermediateCertTemplate, rootCertTemplate, intermediatePrivateKey, rootPrivKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create leaf CA certificate: %v", err)
		}

		intermediatePEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: intermediateCert,
		})

		intermediateCerts = append(intermediateCerts, intermediatePEM...)
	}

	leafIssuer, err := CreateCAIssuer(instance, instance.Spec.Signer.CertificateChain.LeafCA, ctx, deploymentName, client)
	if err != nil {
		return nil, err
	}

	leafCertTemplate, err := generateCertTemplate(*leafIssuer, false, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}, []pkix.Extension{ekuExtension})
	if err != nil {
		return nil, err
	}

	leafPrivKey, err := parsePrivateKey(config.LeafPrivateKey, config.LeafPrivateKeyPassword)
	if err != nil {
		return nil, err
	}

	leafCert, err := createCertificate(leafCertTemplate, rootCertTemplate, leafPrivKey, rootPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create leaf CA certificate: %v", err)
	}

	leafPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: leafCert,
	})

	certificateChain := append(leafPEM, intermediateCerts...)
	certificateChain = append(certificateChain, rootPEM...)

	return certificateChain, nil
}

func GenerateSerialNumber() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, (&big.Int{}).Exp(big.NewInt(2), big.NewInt(159), nil))
	if err != nil {
		return nil, errors.New("error generating serial number")
	}
	return serial, nil
}

func parsePrivateKey(privateKeyPEM []byte, password []byte) (crypto.PrivateKey, error) {
	var err error
	var privateKey crypto.PrivateKey

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	keyBytes := block.Bytes
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		keyBytes, err = x509.DecryptPEMBlock(block, password) //nolint:staticcheck
		if err != nil {
			return nil, err
		}
	}

	if privateKey, err = x509.ParsePKCS8PrivateKey(keyBytes); err != nil {
		if privateKey, err = x509.ParsePKCS1PrivateKey(keyBytes); err != nil {
			if privateKey, err = x509.ParseECPrivateKey(keyBytes); err != nil {
				return nil, fmt.Errorf("failed to parse private key PEM: %w", err)
			}
		}
	}

	switch pk := privateKey.(type) {
	case *rsa.PrivateKey:
		return pk, nil
	case *ecdsa.PrivateKey:
		return pk, nil
	default:
		return nil, fmt.Errorf("unknown private key type")
	}
}

func CreateCAIssuer(instance *rhtasv1alpha1.TimestampAuthority, tsaCA *rhtasv1alpha1.TsaCertificateAuthority, ctx context.Context, deploymentName string, client client.Client) (*Issuer, error) {
	issuer := &Issuer{}
	var err error

	if tsaCA.OrganizationName == "" {
		return nil, fmt.Errorf("could not create certificate: missing OrganizationName from config")
	}

	if tsaCA.CommonName == "" {
		if instance.Spec.ExternalAccess.Enabled {
			if instance.Spec.ExternalAccess.Host != "" {
				issuer.subject.CommonName = instance.Spec.ExternalAccess.Host
			} else {
				if issuer.subject.CommonName, err = kubernetes.CalculateHostname(ctx, client, deploymentName, instance.Namespace); err != nil {
					return nil, err
				}
			}
		} else {
			issuer.subject.CommonName = fmt.Sprintf("%s.%s.svc.local", deploymentName, instance.Namespace)
		}
	} else {
		issuer.subject.CommonName = tsaCA.CommonName
	}

	orgNames := make([]string, 0)
	if tsaCA.OrganizationEmail != "" {
		orgNames = append(orgNames, tsaCA.OrganizationName)
	}
	issuer.subject.Organization = orgNames

	emailAddresses := make([]string, 0)
	if tsaCA.OrganizationEmail != "" {
		emailAddresses = append(emailAddresses, tsaCA.OrganizationEmail)
	}
	issuer.emailAddresses = emailAddresses
	return issuer, nil
}

func generateCertTemplate(issuer Issuer, isCA bool, keyUsage x509.KeyUsage, extKeyUsage []x509.ExtKeyUsage, extraExtensions []pkix.Extension) (x509.Certificate, error) {
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)
	serialNumber, err := GenerateSerialNumber()
	if err != nil {
		return x509.Certificate{}, err
	}

	return x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               issuer.subject,
		EmailAddresses:        issuer.emailAddresses,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              keyUsage,
		ExtKeyUsage:           extKeyUsage,
		ExtraExtensions:       extraExtensions,
		Issuer:                issuer.subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}, nil
}

func createCertificate(certTemplate, parentTemplate x509.Certificate, privKey, rootPrivKey interface{}) ([]byte, error) {
	var cert []byte
	var err error

	switch rootPrivKey := rootPrivKey.(type) {
	case *rsa.PrivateKey:
		rsaPubKey, ok := privKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("invalid private key type for RSA")
		}
		cert, err = x509.CreateCertificate(rand.Reader, &certTemplate, &parentTemplate, &rsaPubKey.PublicKey, rootPrivKey)
	case *ecdsa.PrivateKey:
		ecdsaPubKey, ok := privKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("invalid private key type for ECDSA")
		}
		cert, err = x509.CreateCertificate(rand.Reader, &certTemplate, &parentTemplate, &ecdsaPubKey.PublicKey, rootPrivKey)
	default:
		return nil, fmt.Errorf("unsupported private key type")
	}

	if err != nil {
		return nil, err
	}
	return cert, nil
}
