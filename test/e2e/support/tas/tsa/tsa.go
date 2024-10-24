package tsa

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		WithTransform(condition.IsReady, BeTrue()))

	// server
	Eventually(condition.DeploymentIsRunning(ctx, cli, namespace, "timestamp-authority")).
		Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.TimestampAuthority {
	return func() *v1alpha1.TimestampAuthority {
		instance := &v1alpha1.TimestampAuthority{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{constants.LabelAppComponent: "timestamp-authority", constants.LabelAppName: "tsa-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func GetCertificateChain(ctx context.Context, cli client.Client, ns string, name string, url string) error {
	var resp *http.Response
	var err error
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/api/v1/timestamp/certchain", nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform HTTP request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Println("Error closing response body:", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	err = os.WriteFile("ts_chain.pem", body, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func CreateSecrets(ns string, name string) *v1.Secret {

	config := &tsaUtils.TsaCertChainConfig{
		RootPrivateKeyPassword:          []byte(support.CertPassword),
		IntermediatePrivateKeyPasswords: [][]byte{[]byte(support.CertPassword)},
		LeafPrivateKeyPassword:          []byte(support.CertPassword),
	}
	_, rootPrivateKey, rootCA, err := support.CreateCertificates(true)
	if err != nil {
		return nil
	}
	config.RootPrivateKey = rootPrivateKey

	intermediatePublicKey, intermediatePrivateKey, _, err := support.CreateCertificates(true)
	if err != nil {
		return nil
	}
	config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, intermediatePrivateKey)

	leafPublicKey, leafPrivateKey, _, err := support.CreateCertificates(true)
	if err != nil {
		return nil
	}
	config.LeafPrivateKey = leafPrivateKey

	block, _ := pem.Decode(rootPrivateKey)
	keyBytes := block.Bytes
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		keyBytes, err = x509.DecryptPEMBlock(block, []byte(support.CertPassword)) //nolint:staticcheck
		if err != nil {
			return nil
		}
	}

	rootPrivKey, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(intermediatePublicKey)
	keyBytes = block.Bytes
	interPubKey, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(rootCA)
	rootCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	intermediateCert, err := x509.CreateCertificate(rand.Reader, getCertTemplate(true), rootCert, interPubKey, rootPrivKey)
	if err != nil {
		return nil
	}

	block, _ = pem.Decode(leafPublicKey)
	keyBytes = block.Bytes
	leafPuKey, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil
	}

	leafCert, err := x509.CreateCertificate(rand.Reader, getCertTemplate(false), rootCert, leafPuKey, rootPrivKey)
	if err != nil {
		return nil
	}

	intermediatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: intermediateCert,
	})

	leafPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: leafCert,
	})
	certificateChain := append(leafPEM, intermediatePEM...)
	certificateChain = append(certificateChain, rootCA...)
	config.CertificateChain = certificateChain

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"rootPrivateKey":            config.RootPrivateKey,
			"rootPrivateKeyPassword":    config.RootPrivateKeyPassword,
			"interPrivateKey-0":         config.IntermediatePrivateKeys[0],
			"interPrivateKeyPassword-0": config.IntermediatePrivateKeyPasswords[0],
			"leafPrivateKey":            config.LeafPrivateKey,
			"leafPrivateKeyPassword":    config.LeafPrivateKeyPassword,
			"certificateChain":          config.CertificateChain,
		},
	}
}

func getCertTemplate(isCA bool) *x509.Certificate {
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)
	oidExtendedKeyUsage := asn1.ObjectIdentifier{2, 5, 29, 37}
	oidTimeStamping := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 8}
	ekuValues, err := asn1.Marshal([]asn1.ObjectIdentifier{oidTimeStamping})
	if err != nil {
		return nil
	}

	ekuExtension := pkix.Extension{
		Id:       oidExtendedKeyUsage,
		Critical: true,
		Value:    ekuValues,
	}

	issuer := pkix.Name{
		CommonName:         "local",
		Country:            []string{"CR"},
		Organization:       []string{"RedHat"},
		Province:           []string{"Czech Republic"},
		Locality:           []string{"Brno"},
		OrganizationalUnit: []string{"QE"},
	}

	return &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuer,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		ExtraExtensions:       []pkix.Extension{ekuExtension},
		Issuer:                issuer,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}
}
