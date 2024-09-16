package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TSA", func() {
	Context("TsaSpec", func() {
		It("can be created", func() {
			created := generateTSAObject("tsa-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &TimestampAuthority{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateTSAObject("tsa-updated")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &TimestampAuthority{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Signer.CertificateChain.RootCA = TsaCertificateAuthority{
				CommonName:        "root_test1.com",
				OrganizationName:  "root_test1",
				OrganizationEmail: "root_test1@test.com",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())

			fetched.Spec.Signer.CertificateChain.IntermediateCA[0] = TsaCertificateAuthority{
				CommonName:        "intermediate_test1.com",
				OrganizationName:  "intermediate_test1",
				OrganizationEmail: "intermediate_test1@test.com",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())

			fetched.Spec.Signer.CertificateChain.LeafCA = TsaCertificateAuthority{
				CommonName:        "leaf_test1.com",
				OrganizationName:  "leaf_test1",
				OrganizationEmail: "leaf_test1@test.com",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateTSAObject("tsa-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		It("can be empty", func() {
			created := Securesign{
				Spec: SecuresignSpec{
					TimestampAuthority: nil,
				},
			}
			Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &created))).To(BeFalse())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateTSAObject("tsa-access-1")
				created.Spec.ExternalAccess.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateTSAObject("tsa-access-2")
				created.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing monitoring", func() {
			It("enabled false->true", func() {
				created := generateTSAObject("tsa-monitoring-1")
				created.Spec.Monitoring.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateTSAObject("tsa-monitoring-2")
				created.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &TimestampAuthority{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is Validated", func() {
			It("missing org name for root CA", func() {
				invalidObject := generateTSAObject("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.RootCA.OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName cannot be empty for root certificate authority")))
			})

			It("missing org name for intermediate CA", func() {
				invalidObject := generateTSAObject("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.IntermediateCA[0].OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName cannot be empty for intermediate certificate authority, please make sure all are in place")))
			})

			It("missing org name for leaf CA", func() {
				invalidObject := generateTSAObject("missing-org-name")
				invalidObject.Spec.Signer.CertificateChain.LeafCA.OrganizationName = ""
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("organizationName cannot be empty for leaf certificate authority")))
			})

			It("missing leaf private key", func() {
				invalidObject := generateTSAObject("missing-leaf-private-key")
				invalidObject.Spec.Signer.CertificateChain.RootCA.PrivateKeyRef = &SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: LocalObjectReference{Name: "root-private-key"},
				}
				invalidObject.Spec.Signer.CertificateChain.RootCA.OrganizationName = "root_test1"
				invalidObject.Spec.Signer.CertificateChain.LeafCA.OrganizationName = "leaf_test1"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("must provide private keys for both root and leaf certificate authorities")))
			})

			It("missing root private key", func() {
				invalidObject := generateTSAObject("missing-root-private-key")
				invalidObject.Spec.Signer.CertificateChain.LeafCA.PrivateKeyRef = &SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: LocalObjectReference{Name: "leaf-private-key"},
				}
				invalidObject.Spec.Signer.CertificateChain.RootCA.OrganizationName = "root_test1"
				invalidObject.Spec.Signer.CertificateChain.LeafCA.OrganizationName = "leaf_test1"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("must provide private keys for both root and leaf certificate authorities")))
			})

			It("only cert chain passed in", func() {
				invalidObject := generateTSAObject("just-cert-chain")
				invalidObject.Spec.Signer.CertificateChain.CertificateChainRef = &SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: LocalObjectReference{Name: "leaf-private-key"},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("certificateChainRef should not be present if no signers are configured")))
			})

			It("missing certificate chain for file signer type", func() {
				invalidObject := generateTSAObject("missing-cert-chain")
				invalidObject.Spec.Signer.File = &File{
					PrivateKeyRef: &SecretKeySelector{
						Key:                  "private",
						LocalObjectReference: LocalObjectReference{Name: "private-key-signer"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("signer config needs a matching cert chain in certificateChain.certificateChainRef")))
			})

			It("missing certificate chain for kms signer type", func() {
				invalidObject := generateTSAObject("missing-cert-chain")
				invalidObject.Spec.Signer.Kms = &KMS{
					KeyResource: "kms-resource",
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("signer config needs a matching cert chain in certificateChain.certificateChainRef")))
			})

			It("missing certificate chain for tink signer type", func() {
				invalidObject := generateTSAObject("missing-cert-chain")
				invalidObject.Spec.Signer.Tink = &Tink{
					KeyResource: "tink-resource",
					KeysetRef: &SecretKeySelector{
						Key:                  "tink-resource",
						LocalObjectReference: LocalObjectReference{Name: "tink-resource"},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("signer config needs a matching cert chain in certificateChain.certificateChainRef")))
			})

			It("only one signer is configured at any time", func() {
				invalidObject := generateTSAObject("more-than-one-signer")
				invalidObject.Spec.Signer.Tink = &Tink{
					KeyResource: "tink-resource",
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
		})

	})
})

func generateTSAObject(name string) *TimestampAuthority {
	return &TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: TimestampAuthoritySpec{
			Signer: TimestampAuthoritySigner{
				CertificateChain: CertificateChain{
					RootCA: TsaCertificateAuthority{
						CommonName:        "root_test.com",
						OrganizationName:  "root_test",
						OrganizationEmail: "root_test@test.com",
					},
					IntermediateCA: []TsaCertificateAuthority{
						{
							CommonName:        "intermediate_test.com",
							OrganizationName:  "intermediate_test",
							OrganizationEmail: "intermediate_test@test.com",
						},
					},
					LeafCA: TsaCertificateAuthority{
						CommonName:        "leaf_test.com",
						OrganizationName:  "leaf_test",
						OrganizationEmail: "leaf_test@test.com",
					},
				},
			},
		},
	}
}
