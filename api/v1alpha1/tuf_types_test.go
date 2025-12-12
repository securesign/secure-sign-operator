package v1alpha1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("TUF", func() {

	Context("TufSpec", func() {
		It("can be created", func() {
			created := generateTufObject("tuf-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Tuf{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateTufObject("tuf-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Tuf{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Port = 8080
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateTufObject("tuf-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateTufObject("tuf-access-1")
				created.Spec.ExternalAccess.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Tuf{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateTufObject("tuf-access-2")
				created.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Tuf{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})

			It("edit RouteSelectorLabel", func() {
				created := generateTufObject("tuf-access-3")
				created.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "fake", "foo": "bar"}
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Tuf{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "test", "foo": "bar"}
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("RouteSelectorLabels can't be modified")))
			})
		})

		Context("is validated", func() {
			It("port is negative", func() {
				invalidObject := generateTufObject("port-negative")
				invalidObject.Spec.Port = -20
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("should be greater than or equal to 1")))
			})

			It("port is bigger than 65535", func() {
				invalidObject := generateTufObject("port-large")
				invalidObject.Spec.Port = 65536
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("should be less than or equal to 65535")))
			})

			It("tuf key with unsupported name", func() {
				invalidObject := generateTufObject("unsupported-key")
				invalidObject.Spec.Keys = []TufKey{
					{
						Name: "unsupported",
						SecretRef: &SecretKeySelector{
							LocalObjectReference: LocalObjectReference{
								Name: "fake",
							},
							Key: "fake",
						},
					},
				}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(And(ContainSubstring("Unsupported value:"), ContainSubstring("supported values: \"rekor.pub\", \"ctfe.pub\", \"fulcio_v1.crt.pem\", \"tsa.certchain.pem\""))))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateTufObject("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive with ReadWriteOnce", func() {
					invalidObject := generateTufObject("replicas-positive")
					invalidObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("more than 1 replica, pvc.accessModes must include 'ReadWriteMany'")))
				})

				It("one with ReadWriteOnce", func() {
					validObject := generateTufObject("replicas-single")
					validObject.Spec.Replicas = ptr.To(int32(1))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive with ReadWriteMany", func() {
					validObject := generateTufObject("replicas-positive-rwm")
					validObject.Spec.Replicas = ptr.To(int32(235469))
					validObject.Spec.Pvc.AccessModes = []PersistentVolumeAccessMode{
						PersistentVolumeAccessMode(v1.ReadWriteMany),
					}
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateTufObject("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateTufObject("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
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
			object := generateTufObject("")
			object.GenerateName = "tuf-pvc-"
			object.Spec.Pvc.Name = origObj.name
			object.Spec.Pvc.StorageClass = origObj.storageClass
			object.Spec.Pvc.AccessModes = origObj.accessModes

			err := k8sClient.Create(ctx, object)
			if updateObj == nil && !isValid {
				Expect(err).To(MatchError(ContainSubstring(errMessage)))
				return
			}
			Expect(err).To(Succeed())
			if updateObj == nil {
				return
			}

			object.Spec.Pvc.Name = updateObj.name
			object.Spec.Pvc.StorageClass = updateObj.storageClass
			object.Spec.Pvc.AccessModes = updateObj.accessModes

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
				tufInstance         Tuf
				expectedtufInstance Tuf
			)

			BeforeEach(func() {
				expectedtufInstance = *generateTufObject("foo")
			})

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					tufInstance = Tuf{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tuf-defaults",
							Namespace: "default",
						},
					}

					Expect(k8sClient.Create(context.Background(), &tufInstance)).To(Succeed())
					fetched := &Tuf{}
					Expect(k8sClient.Get(context.Background(), getKey(&tufInstance), fetched)).To(Succeed())
					Expect(fetched.Spec).To(Equal(expectedtufInstance.Spec))
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					tufInstance = Tuf{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tuf-full-manifest",
							Namespace: "default",
						},
						Spec: TufSpec{
							Port: 8181,
							ExternalAccess: ExternalAccess{
								Enabled: true,
								Host:    "hostname",
							},
							Keys: []TufKey{
								{
									Name: "rekor.pub",
									SecretRef: &SecretKeySelector{
										LocalObjectReference: LocalObjectReference{
											Name: "object",
										},
										Key: "key",
									},
								},
							},
						},
					}

					Expect(k8sClient.Create(context.Background(), &tufInstance)).To(Succeed())
					fetchedtuf := &Tuf{}
					Expect(k8sClient.Get(context.Background(), getKey(&tufInstance), fetchedtuf)).To(Succeed())
					Expect(fetchedtuf.Spec).To(Equal(tufInstance.Spec))
				})
			})

			When("CR is partially set", func() {

				It("sets spec.persistence.storage if spec.persistence is partially set", func() {

					tufInstance = Tuf{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tuf-storage",
							Namespace: "default",
						},
						Spec: TufSpec{
							ExternalAccess: ExternalAccess{
								Enabled: true,
							},
						},
					}

					expectedtufInstance.Spec.ExternalAccess.Enabled = true

					Expect(k8sClient.Create(context.Background(), &tufInstance)).To(Succeed())
					fetchedtuf := &Tuf{}
					Expect(k8sClient.Get(context.Background(), getKey(&tufInstance), fetchedtuf)).To(Succeed())
					Expect(fetchedtuf.Spec).To(Equal(expectedtufInstance.Spec))
				})
			})
		})
	})
})

func generateTufObject(name string) *Tuf {
	storage := resource.MustParse("100Mi")
	return &Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: TufSpec{
			PodRequirements: PodRequirements{
				Replicas: ptr.To(int32(1)),
			},
			Port: 80,
			ExternalAccess: ExternalAccess{
				Enabled: false,
			},
			Keys: []TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "ctfe.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
				{
					Name: "tsa.certchain.pem",
				},
			},
			RootKeySecretRef: &LocalObjectReference{
				Name: "tuf-root-keys",
			},
			Pvc: TufPvc{
				Retain: ptr.To(true),
				Size:   &storage,
				AccessModes: []PersistentVolumeAccessMode{
					"ReadWriteOnce",
				},
			},
		},
	}
}
