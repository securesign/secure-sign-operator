package v1alpha1

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Trillian", func() {

	Context("TrillianSpec", func() {
		It("can be created", func() {
			created := generateTrillianObject("trillian-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateTrillianObject("trillian-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Db.Pvc.Name = "new-name"
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateTrillianObject("trillian-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		It("can be created with database secret", func() {
			created := generateTrillianObject("trillian-database-secret")
			created.Spec.Db.DatabaseSecretRef = &LocalObjectReference{
				Name: "database-secret-name",
			}
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())
		})

		Context("is validated", func() {
			It("immutable database create", func() {
				validObject := generateTrillianObject("immutable-create")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Create = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("immutable pvc retain", func() {
				validObject := generateTrillianObject("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Pvc.Retain = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			When("database create", func() {
				It("true", func() {
					By("databaseSecretRef is empty", func() {
						validObject := generateTrillianObject("database-secret-1")
						validObject.Spec.Db.Create = ptr.To(true)
						validObject.Spec.Db.DatabaseSecretRef = nil
						Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
					})
				})

				It("false", func() {
					By("databaseSecretRef is mandatory", func() {
						invalidObject := generateTrillianObject("database-secret-2")
						invalidObject.Spec.Db.Create = ptr.To(false)
						invalidObject.Spec.Db.DatabaseSecretRef = nil
						Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
						Expect(k8sClient.Create(context.Background(), invalidObject)).
							To(MatchError(ContainSubstring("databaseSecretRef cannot be empty")))
					})
				})
			})

			It("checking pvc name", func() {
				invalidObject := generateTrillianObject("trillian3")
				invalidObject.Spec.Db.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.database.pvc.name in body should match")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateTrillianObject("replicas-nil")
					validObject.Spec.LogServer.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateTrillianObject("replicas-positive")
					validObject.Spec.LogServer.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateTrillianObject("replicas-negative")
					invalidObject.Spec.LogServer.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.server.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateTrillianObject("replicas-zero")
					validObject.Spec.LogServer.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		type pvcArgs struct {
			name         string
			storageClass string
			accessModes  []PersistentVolumeAccessMode
		}
		DescribeTable("pvc", func(ctx context.Context, origObj pvcArgs, updateObj *pvcArgs, isValid bool, errMessage string) {
			object := generateTrillianObject("")
			object.GenerateName = "trillian-pvc-"
			object.Spec.Db.Pvc.Name = origObj.name
			object.Spec.Db.Pvc.StorageClass = origObj.storageClass
			object.Spec.Db.Pvc.AccessModes = origObj.accessModes

			err := k8sClient.Create(ctx, object)
			if updateObj == nil && !isValid {
				Expect(err).To(MatchError(ContainSubstring(errMessage)))
				return
			}
			Expect(err).To(Succeed())
			if updateObj == nil {
				return
			}

			object.Spec.Db.Pvc.Name = updateObj.name
			object.Spec.Db.Pvc.StorageClass = updateObj.storageClass
			object.Spec.Db.Pvc.AccessModes = updateObj.accessModes

			if isValid {
				Expect(k8sClient.Update(ctx, object)).To(Succeed())
			} else {
				Expect(k8sClient.Update(ctx, object)).To(MatchError(ContainSubstring(errMessage)))
			}
		},
			Entry("create default", pvcArgs{}, nil, true, ""),
			Entry("bring your own pvc", pvcArgs{name: "byo-pvc"}, nil, true, ""),
			Entry("change name", pvcArgs{}, &pvcArgs{name: "new"}, true, ""),
			Entry("no changes", pvcArgs{storageClass: "default", accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"}}, &pvcArgs{storageClass: "default", accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"}}, true, ""),
			Entry("immutable storageClass", pvcArgs{storageClass: "default"}, &pvcArgs{storageClass: "new"}, false, "storageClass is immutable"),
			Entry("change storageClass when name is set", pvcArgs{name: "named", storageClass: "old"}, &pvcArgs{name: "named", storageClass: "new"}, true, ""),
			Entry("change storageClass and name", pvcArgs{storageClass: "old"}, &pvcArgs{name: "new", storageClass: "new"}, true, ""),
			Entry("immutable accessModes", pvcArgs{accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"}}, &pvcArgs{accessModes: []PersistentVolumeAccessMode{"ReadWriteMany"}}, false, "accessModes is immutable"),
			Entry("change accessModes when name is set", pvcArgs{name: "named", accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"}}, &pvcArgs{name: "named", accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce", "ReadWriteMany"}}, true, ""),
			Entry("change accessModes and name", pvcArgs{accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"}}, &pvcArgs{name: "new", accessModes: []PersistentVolumeAccessMode{"ReadWriteOnce", "ReadWriteMany"}}, true, ""),
		)

		Context("Default settings", func() {
			var (
				trillianInstance         Trillian
				expectedTrillianInstance Trillian
			)

			BeforeEach(func() {
				expectedTrillianInstance = *generateTrillianObject("foo")
			})

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					trillianInstance = Trillian{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "trillian-defaults",
							Namespace: "default",
						},
					}

					Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
					fetched := &Trillian{}
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetched)).To(Succeed())
					Expect(fetched.Spec).To(Equal(expectedTrillianInstance.Spec))
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					storage := k8sresource.MustParse("987Gi")

					trillianInstance = Trillian{
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
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetchedTrillian)).To(Succeed())
					Expect(fetchedTrillian.Spec).To(Equal(trillianInstance.Spec))
				})
			})

			When("CR is partially set", func() {

				It("sets spec.persistence.storage if spec.persistence is partially set", func() {

					trillianInstance = Trillian{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "trillian-storage",
							Namespace: "default",
						},
						Spec: TrillianSpec{
							Db: TrillianDB{
								DatabaseSecretRef: &LocalObjectReference{
									Name: "secret",
								},
							},
						},
					}

					expectedTrillianInstance.Spec.Db.DatabaseSecretRef = &LocalObjectReference{
						Name: "secret",
					}

					Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
					fetchedTrillian := &Trillian{}
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetchedTrillian)).To(Succeed())
					Expect(fetchedTrillian.Spec).To(Equal(expectedTrillianInstance.Spec))
				})
			})
		})
	})
})

func generateTrillianObject(name string) *Trillian {
	storage := k8sresource.MustParse("5Gi")
	return &Trillian{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: TrillianSpec{
			Db: TrillianDB{
				Create: ptr.To(true),
				Pvc: Pvc{
					Retain:      ptr.To(true),
					Size:        &storage,
					AccessModes: []PersistentVolumeAccessMode{"ReadWriteOnce"},
				},
			},
			LogServer: TrillianLogServer{
				PodRequirements: PodRequirements{
					Replicas: ptr.To(int32(1)),
				},
			},
			LogSigner: TrillianLogSigner{
				PodRequirements: PodRequirements{
					Replicas: ptr.To(int32(1)),
				},
			},
			MaxRecvMessageSize: ptr.To(int64(153600)),
		},
	}
}
