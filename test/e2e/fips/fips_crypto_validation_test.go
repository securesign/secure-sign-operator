//go:build fips

package fips

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateEd25519SelfSignedCert() (keyPEM, certPEM []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ed25519-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, nil, err
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return nil, nil, err
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}); err != nil {
		return nil, nil, err
	}

	return keyBuf.Bytes(), certBuf.Bytes(), nil
}

func generateECDSASelfSignedCert(curve elliptic.Curve, sigAlg x509.SignatureAlgorithm) (keyPEM, certPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber:       big.NewInt(1),
		Subject:            pkix.Name{CommonName: "test-ecdsa-ca"},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:               true,
		KeyUsage:           x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		SignatureAlgorithm: sigAlg,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return nil, nil, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return nil, nil, err
	}

	return keyBuf.Bytes(), certBuf.Bytes(), nil
}

var _ = Describe("FIPS crypto material validation", Ordered, func() {
	cli, _ := support.CreateClient()

	Describe("accepts Ed25519 user-provided keys", Ordered, func() {
		var namespace *v1.Namespace

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-compliant-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)

			keyPEM, certPEM, err := generateEd25519SelfSignedCert()
			Expect(err).ToNot(HaveOccurred())

			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-fulcio-secret",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"private": keyPEM,
					"cert":    certPEM,
				},
			}
			Expect(cli.Create(ctx, secret)).To(Succeed())

			s := securesign.Create(namespace.Name, "test-ed25519",
				securesign.WithFipsDefaults(namespace.Name),
				func(s *rhtasv1.Securesign) {
					s.Spec.Fulcio.Certificate = rhtasv1.FulcioCert{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-fulcio-secret"},
							Key:                  "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-fulcio-secret"},
							Key:                  "cert",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("passes FIPS validation for Fulcio cert", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) *metav1.Condition {
				cr := fulcio.Get(ctx, cli, namespace.Name, "test-ed25519")
				if cr == nil {
					return nil
				}
				return meta.FindStatusCondition(cr.GetConditions(), fulcioactions.CertCondition)
			}).WithContext(ctx).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(
				And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				),
			)
		})
	})

	Describe("accepts ECDSA P-384 user-provided keys", Ordered, func() {
		var namespace *v1.Namespace

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-compliant-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)

			keyPEM, certPEM, err := generateECDSASelfSignedCert(elliptic.P384(), x509.ECDSAWithSHA384)
			Expect(err).ToNot(HaveOccurred())

			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-fulcio-secret",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"private": keyPEM,
					"cert":    certPEM,
				},
			}
			Expect(cli.Create(ctx, secret)).To(Succeed())

			s := securesign.Create(namespace.Name, "test-ecdsa",
				securesign.WithFipsDefaults(namespace.Name),
				func(s *rhtasv1.Securesign) {
					s.Spec.Fulcio.Certificate = rhtasv1.FulcioCert{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-fulcio-secret"},
							Key:                  "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-fulcio-secret"},
							Key:                  "cert",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("passes FIPS validation for Fulcio cert", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) *metav1.Condition {
				cr := fulcio.Get(ctx, cli, namespace.Name, "test-ecdsa")
				if cr == nil {
					return nil
				}
				return meta.FindStatusCondition(cr.GetConditions(), fulcioactions.CertCondition)
			}).WithContext(ctx).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(
				And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				),
			)
		})
	})
})
