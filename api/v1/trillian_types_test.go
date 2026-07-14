package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Trillian", func() {

	Context("TrillianSpec", func() {
		It("can be created", func() {
			created := generateMinimalTrillian("trillian-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalTrillian("trillian-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Db.Pvc.Name = "new-name"
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalTrillian("trillian-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		It("can be created with database secret", func() {
			created := generateMinimalTrillian("trillian-database-secret")
			created.Spec.Db.DatabaseSecretRef = &LocalObjectReference{
				Name: "database-secret-name",
			}
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())
		})

		It("default constants are correct", func() {
			created := generateMinimalTrillian("trillian-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.MaxRecvMessageSize).To(Equal(ptr.To(int64(153600))))
			Expect(fetched.Spec.Db.Create).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Db.Provider).To(Equal("mysql"))
			Expect(fetched.Spec.Db.Pvc.Size).To(Equal(ptr.To(k8sresource.MustParse("5Gi"))))
			Expect(fetched.Spec.Db.Pvc.Retain).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.LogServer.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.LogSigner.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.Monitoring.Enabled).To(Equal(ptr.To(true)))
		})

		Context("is validated", func() {
			It("immutable database create", func() {
				validObject := generateMinimalTrillian("immutable-create")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Create = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("immutable pvc retain", func() {
				validObject := generateMinimalTrillian("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Pvc.Retain = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("checking pvc name", func() {
				invalidObject := generateMinimalTrillian("trillian3")
				invalidObject.Spec.Db.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.database.pvc.name in body should match")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalTrillian("replicas-nil")
					validObject.Spec.LogServer.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalTrillian("replicas-positive")
					validObject.Spec.LogServer.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalTrillian("replicas-negative")
					invalidObject.Spec.LogServer.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.server.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalTrillian("replicas-zero")
					validObject.Spec.LogServer.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				storage := k8sresource.MustParse("987Gi")

				trillianInstance := Trillian{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trillian-full-manifest",
						Namespace: "default",
					},
					Spec: TrillianSpec{
						Db: TrillianDB{
							Create: ptr.To(true),
							Pvc: Pvc{
								Retain:       ptr.To(true),
								Name:         "storage",
								StorageClass: "storage-class",
								Size:         &storage,
							},
							DatabaseSecretRef: &LocalObjectReference{
								Name: "secret",
							},
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
				fetchedTrillian := &Trillian{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&trillianInstance), fetchedTrillian)).To(Succeed())
				Expect(fetchedTrillian.Spec).To(Equal(trillianInstance.Spec))
			})
		})
	})
})

func generateMinimalTrillian(name string) *Trillian {
	return &Trillian{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}
