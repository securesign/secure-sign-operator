package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

			fetched.Spec.Signer.CertificateChain.RootCA = &TsaCertificateAuthority{
				CommonName:        "root_test1.com",
				OrganizationName:  "root_test1",
				OrganizationEmail: "root_test1@test.com",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateTSAObject("tsa-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
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

			When("replicas", func() {
				It("nil", func() {
					validObject := generateTSAObject("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateTSAObject("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateTSAObject("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateTSAObject("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
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
			MaxRequestBodySize: ptr.To(int64(1048576)),
		},
	}
}
