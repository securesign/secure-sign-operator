package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fulcio", func() {

	Context("FulcioSpec", func() {
		It("can be created", func() {
			created := generateMinimalFulcio("fulcio-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalFulcio("fulcio-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Config.OIDCIssuers[0] = OIDCIssuer{
				Issuer:   "https://updated.example.com",
				Type:     "email",
				ClientID: "client",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalFulcio("fulcio-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateMinimalFulcio("fulcio-access-1")
				created.Spec.Ingress.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalFulcio("fulcio-access-2")
				created.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})

			It("edit Labels", func() {
				created := generateMinimalFulcio("fulcio-access-3")
				created.Spec.Ingress.Labels = map[string]string{"test": "fake", "foo": "bar"}
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Labels = map[string]string{"test": "test", "foo": "bar"}
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Labels can't be modified")))
			})
		})

		When("changing monitoring", func() {
			It("metrics enabled false->true", func() {
				created := generateMinimalFulcio("fulcio-monitoring-1")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("metrics enabled true->false", func() {
				created := generateMinimalFulcio("fulcio-monitoring-2")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("serviceMonitor requires metrics", func() {
				created := generateMinimalFulcio("fulcio-monitoring-3")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).
					To(MatchError(ContainSubstring("ServiceMonitor requires metrics to be enabled")))
			})
		})

		Context("is validated", func() {
			It("private key", func() {
				invalidObject := generateMinimalFulcio("private-key-invalid")
				invalidObject.Spec.Signer.CertificateChain.CertificateChainRef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("config is not empty", func() {
				invalidObject := generateMinimalFulcio("config-invalid")
				invalidObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				invalidObject.Spec.Config.MetaIssuers = []OIDCIssuer{}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("At least one of oidcIssuers or metaIssuers must be defined")))
			})

			It("CIIssuerMetadata is set", func() {
				validObject := generateMinimalFulcio("config-ci-issuer-metadata")
				addCIIssuerMetadata(validObject)

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("only MetaIssuer is set", func() {
				validObject := generateMinimalFulcio("config-metaissuer")
				validObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				validObject.Spec.Config.MetaIssuers = []OIDCIssuer{
					{
						Issuer:   "https://meta.example.com",
						ClientID: "client",
						Type:     "email",
					},
				}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalFulcio("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalFulcio("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalFulcio("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalFulcio("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalFulcio("fulcio-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.Monitoring.Metrics.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Enabled).To(Equal(ptr.To(false)))
			Expect(fetched.Spec.Ingress.Enabled).To(Equal(ptr.To(false)))
		})

		Context("KMS signer", func() {
			It("valid KMS signer with certificateChainRef", func() {
				validObject := generateMinimalFulcio("fulcio-kms-valid")
				validObject.Spec.Signer = FulcioSigner{
					Type: SignerTypeKMS,
					CertificateChain: FulcioCertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "cert",
							LocalObjectReference: LocalObjectReference{Name: "cert-chain-secret"},
						},
					},
					Kms: &KMS{
						KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer requires certificateChainRef", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-no-chain")
				invalidObject.Spec.Signer = FulcioSigner{
					Type: SignerTypeKMS,
					CertificateChain: FulcioCertificateChain{
						OrganizationName: "org",
					},
					Kms: &KMS{
						KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("certificateChainRef is required")))
			})

			It("KMS signer requires kms field", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-no-kms")
				invalidObject.Spec.Signer = FulcioSigner{
					Type: SignerTypeKMS,
					CertificateChain: FulcioCertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "cert",
							LocalObjectReference: LocalObjectReference{Name: "cert-chain-secret"},
						},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("kms is required")))
			})

			It("invalid KMS URI", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-bad-uri")
				invalidObject.Spec.Signer = FulcioSigner{
					Type: SignerTypeKMS,
					CertificateChain: FulcioCertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "cert",
							LocalObjectReference: LocalObjectReference{Name: "cert-chain-secret"},
						},
					},
					Kms: &KMS{
						KeyResource: "invalid://key",
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("keyResource must be a valid KMS URI")))
			})

			It("file type cannot have kms field", func() {
				invalidObject := generateMinimalFulcio("fulcio-file-with-kms")
				invalidObject.Spec.Signer.Kms = &KMS{
					KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("kms should not be configured")))
			})

			It("valid KMS signer with awskms URI", func() {
				validObject := generateMinimalFulcio("fulcio-kms-aws")
				validObject.Spec.Signer = generateKMSSigner("awskms:///1234abcd-12ab-34cd-56ef-1234567890ab")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("valid KMS signer with awskms ARN URI", func() {
				validObject := generateMinimalFulcio("fulcio-kms-aws-arn")
				validObject.Spec.Signer = generateKMSSigner("awskms:///arn:aws:kms:us-east-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("valid KMS signer with azurekms URI", func() {
				validObject := generateMinimalFulcio("fulcio-kms-azure")
				validObject.Spec.Signer = generateKMSSigner("azurekms://mykeyvault.vault.azure.net/keys/mykey")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("valid KMS signer with hashivault URI", func() {
				validObject := generateMinimalFulcio("fulcio-kms-vault")
				validObject.Spec.Signer = generateKMSSigner("hashivault://cosign")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer with Auth env vars", func() {
				validObject := generateMinimalFulcio("fulcio-kms-auth-env")
				validObject.Spec.Signer = generateKMSSigner("gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				validObject.Spec.Auth = &Auth{
					Env: []corev1.EnvVar{
						{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: "/var/run/secrets/gcp/sa.json"},
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer with Auth secretMount", func() {
				validObject := generateMinimalFulcio("fulcio-kms-auth-mount")
				validObject.Spec.Signer = generateKMSSigner("awskms:///1234abcd-12ab-34cd-56ef-1234567890ab")
				validObject.Spec.Auth = &Auth{
					SecretMount: []SecretKeySelector{
						{Key: "credentials", LocalObjectReference: LocalObjectReference{Name: "aws-creds"}},
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer with Auth env and secretMount combined", func() {
				validObject := generateMinimalFulcio("fulcio-kms-auth-both")
				validObject.Spec.Signer = generateKMSSigner("azurekms://mykeyvault.vault.azure.net/keys/mykey")
				validObject.Spec.Auth = &Auth{
					Env: []corev1.EnvVar{
						{Name: "AZURE_TENANT_ID", Value: "tenant"},
						{Name: "AZURE_CLIENT_ID", Value: "client"},
					},
					SecretMount: []SecretKeySelector{
						{Key: "client-secret", LocalObjectReference: LocalObjectReference{Name: "azure-sp"}},
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer with empty Auth", func() {
				validObject := generateMinimalFulcio("fulcio-kms-auth-empty")
				validObject.Spec.Signer = generateKMSSigner("hashivault://cosign")
				validObject.Spec.Auth = &Auth{}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("KMS signer can be deleted", func() {
				created := generateMinimalFulcio("fulcio-kms-delete")
				created.Spec.Signer = generateKMSSigner("gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
			})

			It("KMS signer fully populated", func() {
				fulcio := Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fulcio-kms-full",
						Namespace: "default",
					},
					Spec: FulcioSpec{
						Monitoring: MonitoringConfig{
							Metrics:        MetricsConfig{Enabled: ptr.To(true)},
							ServiceMonitor: ServiceMonitorConfig{Enabled: ptr.To(true)},
						},
						Ingress: Ingress{
							Enabled: ptr.To(true),
							Host:    "fulcio.example.com",
						},
						Config: FulcioConfig{
							OIDCIssuers: []OIDCIssuer{
								{Issuer: "https://issuer.example.com", ClientID: "client", Type: "email"},
							},
						},
						Signer: FulcioSigner{
							Type: SignerTypeKMS,
							CertificateChain: FulcioCertificateChain{
								CertificateChainRef: &SecretKeySelector{
									Key:                  "cert-chain",
									LocalObjectReference: LocalObjectReference{Name: "fulcio-cert-chain"},
								},
							},
							Kms: &KMS{
								KeyResource: "awskms:///arn:aws:kms:us-east-1:123456789012:key/mrk-abc123",
							},
						},
						Auth: &Auth{
							Env: []corev1.EnvVar{
								{Name: "AWS_REGION", Value: "us-east-1"},
							},
							SecretMount: []SecretKeySelector{
								{Key: "credentials", LocalObjectReference: LocalObjectReference{Name: "aws-kms-creds"}},
							},
						},
						Ctlog: ServiceReference{},
					},
				}
				Expect(k8sClient.Create(context.Background(), &fulcio)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&fulcio), fetched)).To(Succeed())
				Expect(fetched.Spec).To(Equal(fulcio.Spec))
			})

			It("KMS type cannot have file field", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-with-file")
				invalidObject.Spec.Signer = generateKMSSigner("gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				invalidObject.Spec.Signer.File = &FulcioFile{
					PrivateKeyRef: &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "secret"}},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("file should not be configured")))
			})

			It("empty KMS keyResource", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-empty-key")
				invalidObject.Spec.Signer = FulcioSigner{
					Type: SignerTypeKMS,
					CertificateChain: FulcioCertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "cert",
							LocalObjectReference: LocalObjectReference{Name: "cert-chain-secret"},
						},
					},
					Kms: &KMS{
						KeyResource: "",
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("keyResource must be a valid KMS URI")))
			})

			It("KMS URI scheme only without path", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-scheme-only")
				invalidObject.Spec.Signer = generateKMSSigner("gcpkms://")
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("keyResource must be a valid KMS URI")))
			})

			It("KMS URI is case sensitive", func() {
				invalidObject := generateMinimalFulcio("fulcio-kms-uppercase")
				invalidObject.Spec.Signer = generateKMSSigner("GCPKMS://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("keyResource must be a valid KMS URI")))
			})

			It("default type with kms field rejected", func() {
				invalidObject := generateMinimalFulcio("fulcio-default-with-kms")
				invalidObject.Spec.Signer.Type = ""
				invalidObject.Spec.Signer.Kms = &KMS{
					KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("kms should not be configured")))
			})

			It("update file signer to KMS", func() {
				created := generateMinimalFulcio("fulcio-file-to-kms")
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())

				fetched.Spec.Signer = generateKMSSigner("gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("update KMS signer to file", func() {
				created := generateMinimalFulcio("fulcio-kms-to-file")
				created.Spec.Signer = generateKMSSigner("gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k")
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())

				fetched.Spec.Signer = FulcioSigner{
					Type: SignerTypeFile,
					CertificateChain: FulcioCertificateChain{
						OrganizationName: "org",
					},
				}
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				fulcioInstance := Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fulcio-full-manifest",
						Namespace: "default",
					},
					Spec: FulcioSpec{
						Monitoring: MonitoringConfig{
							Metrics:        MetricsConfig{Enabled: ptr.To(true)},
							ServiceMonitor: ServiceMonitorConfig{Enabled: ptr.To(true)},
						},
						Ingress: Ingress{
							Enabled: ptr.To(true),
							Host:    "hostname",
						},
						Config: FulcioConfig{
							OIDCIssuers: []OIDCIssuer{
								{
									Issuer:            "https://issuer1.example.com",
									ClientID:          "client",
									Type:              "email",
									IssuerURL:         "https://issuer1.example.com",
									IssuerClaim:       "claim",
									ChallengeClaim:    "challenge",
									SPIFFETrustDomain: "SPIFFE",
									SubjectDomain:     "domain",
								},
								{
									Issuer:            "https://issuer2.example.com",
									ClientID:          "client2",
									Type:              "username",
									IssuerURL:         "https://issuer2.example.com",
									IssuerClaim:       "claim2",
									ChallengeClaim:    "challenge2",
									SPIFFETrustDomain: "SPIFFE2",
									SubjectDomain:     "domain2",
								},
							},
						},
						Signer: FulcioSigner{
							Type: "file",
							CertificateChain: FulcioCertificateChain{
								CommonName:          "CommonName",
								OrganizationName:    "OrganizationName",
								OrganizationEmail:   "OrganizationEmail",
								CertificateChainRef: &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "name"}},
							},
							File: &FulcioFile{
								PrivateKeyRef: &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "name"}},
							},
						},
						Ctlog: ServiceReference{
							URL: "http://ctlog.default.svc:80/trusted-artifact-signer",
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &fulcioInstance)).To(Succeed())
				fetchedFulcio := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&fulcioInstance), fetchedFulcio)).To(Succeed())
				Expect(fetchedFulcio.Spec).To(Equal(fulcioInstance.Spec))
			})
		})
	})
})

func generateMinimalFulcio(name string) *Fulcio {
	return &Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: FulcioSpec{
			Config: FulcioConfig{
				OIDCIssuers: []OIDCIssuer{
					{
						ClientID:   "client",
						Type:       "email",
						IssuerURL:  "https://issuer1.example.com",
						Issuer:     "https://issuer1.example.com",
						CIProvider: "foo",
					},
					{
						ClientID:   "ci-client",
						Type:       "ci-provider",
						CIProvider: "foo",
						IssuerURL:  "https://issuer2.example.com",
						Issuer:     "https://issuer2.example.com",
					},
				},
				MetaIssuers: []OIDCIssuer{
					{
						ClientID:  "client",
						Type:      "email",
						IssuerURL: "https://meta1.example.com",
						Issuer:    "https://meta1.example.com",
					},
					{
						ClientID: "client",
						Type:     "email",
						Issuer:   "https://meta2.example.com",
					},
				},
			},
			Signer: FulcioSigner{
				Type: "file",
				CertificateChain: FulcioCertificateChain{
					CommonName:       "hostname",
					OrganizationName: "organization",
				},
			},
		},
	}
}

func generateKMSSigner(keyResource string) FulcioSigner {
	return FulcioSigner{
		Type: SignerTypeKMS,
		CertificateChain: FulcioCertificateChain{
			CertificateChainRef: &SecretKeySelector{
				Key:                  "cert",
				LocalObjectReference: LocalObjectReference{Name: "cert-chain-secret"},
			},
		},
		Kms: &KMS{
			KeyResource: keyResource,
		},
	}
}

func addCIIssuerMetadata(config *Fulcio) *Fulcio {
	config.Spec.Config.CIIssuerMetadata = []CIIssuerMetadata{
		{
			IssuerName:                     "gitlab-ci",
			DefaultTemplateValues:          map[string]string{"url": "https://gitlab.com"},
			SubjectAlternativeNameTemplate: "https://{{ .ci_config_ref_uri }}",
			ExtensionTemplates: Extensions{
				BuildSignerURI:                      "https://{{ .ci_config_ref_uri }}",
				BuildSignerDigest:                   "ci_config_sha",
				RunnerEnvironment:                   "runner_environment",
				SourceRepositoryURI:                 "{{ .url }}/{{ .project_path }}",
				SourceRepositoryDigest:              "sha",
				SourceRepositoryRef:                 "refs/{{if eq .ref_type \"branch\"}}heads/{{ else }}tags/{{end}}{{ .ref }}",
				SourceRepositoryIdentifier:          "project_id",
				SourceRepositoryOwnerURI:            "{{ .url }}/{{ .namespace_path }}",
				SourceRepositoryOwnerIdentifier:     "namespace_id",
				BuildConfigURI:                      "https://{{ .ci_config_ref_uri }}",
				BuildConfigDigest:                   "ci_config_sha",
				BuildTrigger:                        "pipeline_source",
				RunInvocationURI:                    "{{ .url }}/{{ .project_path }}/-/jobs/{{ .job_id }}",
				SourceRepositoryVisibilityAtSigning: "project_visibility",
			},
		},
	}
	return config
}
