package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CTlog", func() {

	Context("CTlogSpec", func() {
		It("can be created", func() {
			created := generateMinimalCTlog("ctlog-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalCTlog("ctlog-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalCTlog("ctlog-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		It("webhook defaults match SetDefaults", func() {
			created := generateMinimalCTlog("ctlog-defaults")
			expected := generateMinimalCTlog("ctlog-defaults")
			expected.Spec.SetDefaults()

			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec).To(Equal(expected.Spec))
		})

		Context("is validated", func() {
			It("public key", func() {
				invalidObject := generateMinimalCTlog("public-key-invalid")
				invalidObject.Spec.PublicKeyRef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("private key password", func() {
				invalidObject := generateMinimalCTlog("private-key-password-invalid")
				invalidObject.Spec.PublicKeyRef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalCTlog("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalCTlog("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalCTlog("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalCTlog("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				tree := int64(1269875)
				port := int32(8091)
				ctlogInstance := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-full-manifest",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						PublicKeyRef: &SecretKeySelector{
							Key: "key",
							LocalObjectReference: LocalObjectReference{
								Name: "name",
							},
						},
						PrivateKeyRef: &SecretKeySelector{
							Key: "key",
							LocalObjectReference: LocalObjectReference{
								Name: "name",
							},
						},
						PrivateKeyPasswordRef: &SecretKeySelector{
							Key: "key",
							LocalObjectReference: LocalObjectReference{
								Name: "name",
							},
						},
						RootCertificates: []SecretKeySelector{
							{
								Key: "key",
								LocalObjectReference: LocalObjectReference{
									Name: "name",
								},
							},
						},
						Trillian: TrillianService{
							Address: "trillian-system.default.svc",
							Port:    &port,
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
				fetchedCTlog := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&ctlogInstance), fetchedCTlog)).To(Succeed())
				Expect(fetchedCTlog.Spec).To(Equal(ctlogInstance.Spec))
			})
		})
	})
})

func generateMinimalCTlog(name string) *CTlog {
	return &CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}
