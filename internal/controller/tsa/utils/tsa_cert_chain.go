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
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TsaCertChainConfig struct {
	RootPrivateKey          []byte
	RootPrivateKeyPassword  []byte
	InterPrivateKey         []byte
	InterPrivateKeyPassword []byte
	CertificateChain        []byte
}

type issuer struct {
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
	if len(c.InterPrivateKey) > 0 {
		result["interPrivateKey"] = c.InterPrivateKey
	}
	if len(c.InterPrivateKeyPassword) > 0 {
		result["interPrivateKeyPassword"] = c.InterPrivateKeyPassword
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

func CreateTSACertChain(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority, deploymentName string, client client.Client, config *TsaCertChainConfig) ([]byte, error) {
	var err error

	rootIssuer, err := CreateCAIssuer(instance, &instance.Spec.Signer.CertificateChain.RootCA, ctx, deploymentName, client)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)
	rootSerialNumber, err := GenerateSerialNumber()
	if err != nil {
		return nil, err
	}

	rootCertTemplate := x509.Certificate{
		SerialNumber:          rootSerialNumber,
		Subject:               rootIssuer.subject,
		EmailAddresses:        rootIssuer.emailAddresses,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Issuer:                rootIssuer.subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}

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

	interSerialNumber, err := GenerateSerialNumber()
	if err != nil {
		return nil, err
	}

	intermediateIssuer, err := CreateCAIssuer(instance, &instance.Spec.Signer.CertificateChain.RootCA, ctx, deploymentName, client)
	if err != nil {
		return nil, err
	}

	intermediateCertTemplate := x509.Certificate{
		SerialNumber:          interSerialNumber,
		Subject:               intermediateIssuer.subject,
		EmailAddresses:        intermediateIssuer.emailAddresses,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		ExtraExtensions:       []pkix.Extension{ekuExtension},
		Issuer:                intermediateIssuer.subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}

	rootPrivKey, err := parsePrivateKey(config.RootPrivateKey, config.RootPrivateKeyPassword)
	if err != nil {
		return nil, err
	}

	interPrivKey, err := parsePrivateKey(config.InterPrivateKey, config.InterPrivateKeyPassword)
	if err != nil {
		return nil, err
	}

	var rootCert []byte
	var intermediateCert []byte
	switch rootPrivKey := rootPrivKey.(type) {
	case *rsa.PrivateKey:
		if interPrivKey, ok := interPrivKey.(*rsa.PrivateKey); ok {
			rootCert, intermediateCert, err = createRootAndIntermediateCertificates(rootCertTemplate, intermediateCertTemplate, rootPrivKey, rootPrivKey.Public(), interPrivKey.Public())
			if err != nil {
				return nil, fmt.Errorf("Failed to create root and intermediate CA: %s", err)
			}
		} else {
			return nil, fmt.Errorf("intermediate private key is not of type *rsa.PrivateKey")
		}
	case *ecdsa.PrivateKey:
		if interPrivKey, ok := interPrivKey.(*ecdsa.PrivateKey); ok {
			rootCert, intermediateCert, err = createRootAndIntermediateCertificates(rootCertTemplate, intermediateCertTemplate, rootPrivKey, rootPrivKey.Public(), interPrivKey.Public())
			if err != nil {
				return nil, fmt.Errorf("Failed to create root and intermediate CA: %s", err)
			}
		} else {
			return nil, fmt.Errorf("intermediate private key is not of type *ecdsa.PrivateKey")
		}
	default:
		return nil, fmt.Errorf("unknown private key type")
	}

	rootPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rootCert,
	})

	intermediatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: intermediateCert,
	})

	certificateChain := append(intermediatePEM, rootPEM...)
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
	if x509.IsEncryptedPEMBlock(block) {
		keyBytes, err = x509.DecryptPEMBlock(block, password)
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

func createRootAndIntermediateCertificates(rootCertTemplate, intermediateCertTemplate x509.Certificate, rootPrivKey, rootPubKey, interPubKey any) ([]byte, []byte, error) {
	rootCert, err := x509.CreateCertificate(rand.Reader, &rootCertTemplate, &rootCertTemplate, rootPubKey, rootPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create root CA certificate: %v", err)
	}

	intermediateCert, err := x509.CreateCertificate(rand.Reader, &intermediateCertTemplate, &rootCertTemplate, interPubKey, rootPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create intermediate CA certificate: %v", err)
	}

	return rootCert, intermediateCert, nil
}

func CreateCAIssuer(instance *rhtasv1alpha1.TimestampAuthority, tsaCA *rhtasv1alpha1.TsaCertificateAuthority, ctx context.Context, deploymentName string, client client.Client) (*issuer, error) {
	issuer := &issuer{}
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
