package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TSA", func() {
	Context("TsaSpec", func() {
		It("can be created", func() {
			created := generateMinimalTSA("tsa-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &TimestampAuthority{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalTSA("tsa-updated")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &TimestampAuthority{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Signer.CertificateChain.RootCA = &TsaCertificateAuthority{
				CommonName:        "root_test1.com",
				OrganizationName:  "root_test1",
				OrganizationEmail: "root_test1@test.com",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalTSA("tsa-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateMinimalTSA("tsa-access-1")
				created.Spec.ExternalAccess.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalTSA("tsa-access-2")
				created.Spec.ExternalAccess.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing monitoring", func() {
			It("metrics enabled false->true", func() {
				created := generateMinimalTSA("tsa-monitoring-1")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("metrics enabled true->false", func() {
				created := generateMinimalTSA("tsa-monitoring-2")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("serviceMonitor requires metrics", func() {
				created := generateMinimalTSA("tsa-monitoring-3")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).
					To(MatchError(ContainSubstring("ServiceMonitor requires metrics to be enabled")))
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalTSA("tsa-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &TimestampAuthority{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.MaxRequestBodySize).To(Equal(ptr.To(int64(1048576))))
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.NTPMonitoring.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Monitoring.Metrics.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Enabled).To(Equal(ptr.To(false)))
			Expect(fetched.Spec.ExternalAccess.Enabled).To(Equal(ptr.To(false)))
		})

		Context("is Validated", func() {
			It("missing org name for root CA", func() {
				invalidObject := generateMinimalTSA("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.RootCA.OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName in body should be at least 1 chars long")))
			})

			It("missing org name for intermediate CA", func() {
				invalidObject := generateMinimalTSA("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.IntermediateCA[0].OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName in body should be at least 1 chars long")))
			})

			It("missing org name for leaf CA", func() {
				invalidObject := generateMinimalTSA("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.LeafCA.OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName in body should be at least 1 chars long")))
			})

			It("only one signer is configured at any time", func() {
				invalidObject := generateMinimalTSA("more-than-one-signer")
				invalidObject.Spec.Signer.CertificateChain = CertificateChain{
					CertificateChainRef: &SecretKeySelector{
						Key:                  "chain",
						LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
					},
				}
				invalidObject.Spec.Signer.Tink = &Tink{
					KeyResource: "gcp-kms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
					KeysetRef: &SecretKeySelector{
						Key:                  "tink-resource",
						LocalObjectReference: LocalObjectReference{Name: "tink-resource"},
					},
				}
				invalidObject.Spec.Signer.File = &File{
					PrivateKeyRef: &SecretKeySelector{
						Key:                  "private",
						LocalObjectReference: LocalObjectReference{Name: "private-key-signer"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("only one signer should be configured at any time")))
			})

			It("signer requires certificateChainRef", func() {
				invalidObject := generateMinimalTSA("signer-no-chainref")
				invalidObject.Spec.Signer.File = &File{
					PrivateKeyRef: &SecretKeySelector{
						Key:                  "private",
						LocalObjectReference: LocalObjectReference{Name: "private-key-signer"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("external signer (file/kms/tink) and certificateChainRef must be configured together")))
			})

			It("certificateChainRef requires signer", func() {
				invalidObject := generateMinimalTSA("chainref-no-signer")
				invalidObject.Spec.Signer.CertificateChain = CertificateChain{
					CertificateChainRef: &SecretKeySelector{
						Key:                  "chain",
						LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("external signer (file/kms/tink) and certificateChainRef must be configured together")))
			})

			It("certificateChainRef excludes CA fields", func() {
				invalidObject := generateMinimalTSA("chainref-with-ca")
				invalidObject.Spec.Signer.CertificateChain.CertificateChainRef = &SecretKeySelector{
					Key:                  "chain",
					LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
				}
				invalidObject.Spec.Signer.File = &File{
					PrivateKeyRef: &SecretKeySelector{
						Key:                  "private",
						LocalObjectReference: LocalObjectReference{Name: "private-key-signer"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("rootCA/leafCA/intermediateCA must not be set when certificateChainRef is provided")))
			})

			It("file signer requires privateKeyRef", func() {
				invalidObject := generateMinimalTSA("file-no-key")
				invalidObject.Spec.Signer.CertificateChain = CertificateChain{
					CertificateChainRef: &SecretKeySelector{
						Key:                  "chain",
						LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
					},
				}
				invalidObject.Spec.Signer.File = &File{}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef")))
			})

			It("valid file signer with certificateChainRef", func() {
				validObject := generateMinimalTSA("valid-file-signer")
				validObject.Spec.Signer = TimestampAuthoritySigner{
					CertificateChain: CertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "chain",
							LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
						},
					},
					File: &File{
						PrivateKeyRef: &SecretKeySelector{
							Key:                  "private",
							LocalObjectReference: LocalObjectReference{Name: "private-key-signer"},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("missing intermediateCA", func() {
				invalidObject := generateMinimalTSA("missing-intermediate")
				invalidObject.Spec.Signer.CertificateChain.IntermediateCA = nil
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("rootCA/leafCA/intermediateCA are all required when certificateChainRef is not provided")))
			})

			It("empty intermediateCA array", func() {
				invalidObject := generateMinimalTSA("empty-intermediate")
				invalidObject.Spec.Signer.CertificateChain.IntermediateCA = []*TsaCertificateAuthority{}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("rootCA/leafCA/intermediateCA are all required when certificateChainRef is not provided")))
			})

			It("valid KMS signer with certificateChainRef", func() {
				validObject := generateMinimalTSA("valid-kms-signer")
				validObject.Spec.Signer = TimestampAuthoritySigner{
					CertificateChain: CertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "chain",
							LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
						},
					},
					Kms: &KMS{
						KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("valid Tink signer with certificateChainRef", func() {
				validObject := generateMinimalTSA("valid-tink-signer")
				validObject.Spec.Signer = TimestampAuthoritySigner{
					CertificateChain: CertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "chain",
							LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
						},
					},
					Tink: &Tink{
						KeyResource: "gcp-kms://projects/p/locations/l/keyRings/kr/cryptoKeys/k",
						KeysetRef: &SecretKeySelector{
							Key:                  "keyset",
							LocalObjectReference: LocalObjectReference{Name: "tink-keyset"},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("invalid KMS URI", func() {
				invalidObject := generateMinimalTSA("invalid-kms-uri")
				invalidObject.Spec.Signer = TimestampAuthoritySigner{
					CertificateChain: CertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "chain",
							LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
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

			It("invalid Tink URI", func() {
				invalidObject := generateMinimalTSA("invalid-tink-uri")
				invalidObject.Spec.Signer = TimestampAuthoritySigner{
					CertificateChain: CertificateChain{
						CertificateChainRef: &SecretKeySelector{
							Key:                  "chain",
							LocalObjectReference: LocalObjectReference{Name: "chain-secret"},
						},
					},
					Tink: &Tink{
						KeyResource: "invalid://key",
						KeysetRef: &SecretKeySelector{
							Key:                  "keyset",
							LocalObjectReference: LocalObjectReference{Name: "tink-keyset"},
						},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("keyResource must be a valid Tink KMS URI")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalTSA("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalTSA("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalTSA("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalTSA("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})
	})
})

func generateMinimalTSA(name string) *TimestampAuthority {
	return &TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: TimestampAuthoritySpec{
			Signer: TimestampAuthoritySigner{
				CertificateChain: CertificateChain{
					RootCA: &TsaCertificateAuthority{
						CommonName:        "root_test.com",
						OrganizationName:  "root_test",
						OrganizationEmail: "root_test@test.com",
					},
					IntermediateCA: []*TsaCertificateAuthority{
						{
							CommonName:        "intermediate_test.com",
							OrganizationName:  "intermediate_test",
							OrganizationEmail: "intermediate_test@test.com",
						},
					},
					LeafCA: &TsaCertificateAuthority{
						CommonName:        "leaf_test.com",
						OrganizationName:  "leaf_test",
						OrganizationEmail: "leaf_test@test.com",
					},
				},
			},
		},
	}
}
