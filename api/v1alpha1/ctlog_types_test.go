package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CTlog", func() {

	Context("CTlogSpec", func() {
		It("can be created", func() {
			created := generateCTlogObject("ctlog-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateCTlogObject("ctlog-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateCTlogObject("ctlog-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		When("changing External Ctlog setting", func() {
			It("enabled false->true", func() {
				created := generateCTlogObject("ctlog-access-1")
				created.Spec.ExternalCtlog.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &CTlog{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalCtlog.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateCTlogObject("ctlog-access-2")
				created.Spec.ExternalCtlog.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &CTlog{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalCtlog.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is validated", func() {
			It("public key", func() {
				invalidObject := generateCTlogObject("public-key-invalid")
				invalidObject.Spec.PublicKeyRef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("private key password", func() {
				invalidObject := generateCTlogObject("private-key-password-invalid")
				invalidObject.Spec.PublicKeyRef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})
		})

		Context("Default settings", func() {
			var (
				ctlogInstance         CTlog
				expectedCTlogInstance CTlog
			)

			BeforeEach(func() {
				expectedCTlogInstance = *generateCTlogObject("foo")
			})

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					ctlogInstance = CTlog{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ctlog-defaults",
							Namespace: "default",
						},
					}

					Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
					fetched := &CTlog{}
					Expect(k8sClient.Get(context.Background(), getKey(&ctlogInstance), fetched)).To(Succeed())
					Expect(fetched.Spec).To(Equal(expectedCTlogInstance.Spec))
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					tree := int64(1269875)
					ctlogInstance = CTlog{
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
							ExternalCtlog: ExternalCtlog{
								Enabled: true,
							},
						},
					}

					Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
					fetchedCTlog := &CTlog{}
					Expect(k8sClient.Get(context.Background(), getKey(&ctlogInstance), fetchedCTlog)).To(Succeed())
					Expect(fetchedCTlog.Spec).To(Equal(ctlogInstance.Spec))
				})
			})

			When("CR is partially set", func() {

				It("sets spec.pvc.storage if spec.pvc is partially set", func() {
					tree := int64(1269875)
					ctlogInstance = CTlog{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ctlog-storage",
							Namespace: "default",
						},
						Spec: CTlogSpec{
							TreeID: &tree,
						},
					}

					expectedCTlogInstance.Spec.TreeID = &tree
					Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
					fetchedCTlog := &CTlog{}
					Expect(k8sClient.Get(context.Background(), getKey(&ctlogInstance), fetchedCTlog)).To(Succeed())
					Expect(fetchedCTlog.Spec).To(Equal(expectedCTlogInstance.Spec))
				})
			})
		})
	})
})

func generateCTlogObject(name string) *CTlog {
	return &CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: CTlogSpec{},
	}
}
