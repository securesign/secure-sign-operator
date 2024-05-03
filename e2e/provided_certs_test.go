//go:build integration

package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"github.com/securesign/operator/controllers/common/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/e2e/support"
	"github.com/securesign/operator/e2e/support/tas"
	clients "github.com/securesign/operator/e2e/support/tas/cli"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const CertPassword = "LetMeIn123"

var _ = Describe("Securesign install with provided certs", Ordered, func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	var targetImageName string
	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			if val, present := os.LookupEnv("CI"); present && val == "true" {
				support.DumpNamespace(ctx, cli, namespace.Name)
			}
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			cli.Delete(ctx, namespace)
		})

		securesign = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": "false",
				},
			},
			Spec: v1alpha1.SecuresignSpec{
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Signer: v1alpha1.RekorSigner{
						KMS: "secret",
						KeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-rekor-secret",
							},
							Key: "private",
						},
					},
				},
				Fulcio: v1alpha1.FulcioSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Config: v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: v1alpha1.FulcioCert{
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "private",
						},
						PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "password",
						},
						CARef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
				},
				Ctlog: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-ctlog-secret",
						},
						Key: "private",
					},
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
				},
				Tuf: v1alpha1.TufSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Keys: []v1alpha1.TufKey{
						{
							Name: "fulcio_v1.crt.pem",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-fulcio-secret",
								},
								Key: "cert",
							},
						},
						{
							Name: "rekor.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-rekor-secret",
								},
								Key: "public",
							},
						},
						{
							Name: "ctfe.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-ctlog-secret",
								},
								Key: "public",
							},
						},
					},
				},
				Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
					Create: utils.Pointer(true),
				}},
			},
		}
	})

	BeforeAll(func() {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with provided certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, initCTSecret(namespace.Name, "my-ctlog-secret")))
			Expect(cli.Create(ctx, initFulcioSecret(namespace.Name, "my-fulcio-secret")))
			Expect(cli.Create(ctx, initRekorSecret(namespace.Name, "my-rekor-secret")))
			Expect(cli.Create(ctx, securesign)).To(Succeed())
		})

		It("fulcio is running with mounted certs", func() {
			tas.VerifyFulcio(ctx, cli, namespace.Name, securesign.Name)
			server := tas.GetFulcioServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			sp := []v1.SecretProjection{}
			for _, volume := range server.Spec.Volumes {
				if volume.Name == "fulcio-cert" {
					for _, source := range volume.VolumeSource.Projected.Sources {
						sp = append(sp, *source.Secret)
					}
				}
			}

			Expect(sp).To(
				ContainElement(
					WithTransform(func(sp v1.SecretProjection) string {
						return sp.Name
					}, Equal("my-fulcio-secret")),
				))

		})

		It("rekor is running with mounted certs", func() {
			tas.VerifyRekor(ctx, cli, namespace.Name, securesign.Name)
			server := tas.GetRekorServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.VolumeSource.Secret != nil {
							return volume.VolumeSource.Secret.SecretName
						}
						return ""
					}, Equal("my-rekor-secret")),
				))

		})

		It("All other components are running", func() {
			tas.VerifySecuresign(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyCTLog(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTrillian(ctx, cli, namespace.Name, securesign.Name, true)
			tas.VerifyTuf(ctx, cli, namespace.Name, securesign.Name)
		})

		It("Use cosign cli", func() {
			fulcio := tas.GetFulcio(ctx, cli, namespace.Name, securesign.Name)()
			Expect(fulcio).ToNot(BeNil())

			rekor := tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)()
			Expect(rekor).ToNot(BeNil())

			tuf := tas.GetTuf(ctx, cli, namespace.Name, securesign.Name)()
			Expect(tuf).ToNot(BeNil())

			oidcToken, err := support.OidcToken(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(oidcToken).ToNot(BeEmpty())

			// sleep for a while to be sure everything has settled down
			time.Sleep(time.Duration(10) * time.Second)

			Expect(clients.Execute("cosign", "initialize", "--mirror="+tuf.Status.Url, "--root="+tuf.Status.Url+"/root.json")).To(Succeed())

			Expect(clients.Execute(
				"cosign", "sign", "-y",
				"--fulcio-url="+fulcio.Status.Url,
				"--rekor-url="+rekor.Status.Url,
				"--oidc-issuer="+support.OidcIssuerUrl(),
				"--oidc-client-id="+support.OidcClientID(),
				"--identity-token="+oidcToken,
				targetImageName,
			)).To(Succeed())

			Expect(clients.Execute(
				"cosign", "verify",
				"--rekor-url="+rekor.Status.Url,
				"--certificate-identity-regexp", ".*@redhat",
				"--certificate-oidc-issuer-regexp", ".*keycloak.*",
				targetImageName,
			)).To(Succeed())
		})
	})
})

func initFulcioSecret(ns string, name string) *v1.Secret {
	public, private, root, err := initCertificates(true)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"password": []byte(CertPassword),
			"private":  private,
			"public":   public,
			"cert":     root,
		},
	}
}

func initRekorSecret(ns string, name string) *v1.Secret {
	public, private, _, err := initCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

func initCTSecret(ns string, name string) *v1.Secret {
	public, private, _, err := initCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

func initCertificates(passwordProtected bool) ([]byte, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	// private
	privateKeyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	var block *pem.Block
	if passwordProtected {
		block, err = x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", privateKeyBytes, []byte(CertPassword), x509.PEMCipher3DES)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		block = &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privateKeyBytes,
		}
	}
	privateKeyPem := pem.EncodeToMemory(block)

	// public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	publicKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: publicKeyBytes,
		},
	)

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:         "local",
		Country:            []string{"CR"},
		Organization:       []string{"RedHat"},
		Province:           []string{"Czech Republic"},
		Locality:           []string{"Brno"},
		OrganizationalUnit: []string{"QE"},
	}
	//Create certificate templet
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               issuer,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Issuer:                issuer,
	}
	//Create certificate using templet
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err

	}
	//pem encoding of certificate
	root := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: derBytes,
		},
	)
	return publicKeyPem, privateKeyPem, root, err
}
