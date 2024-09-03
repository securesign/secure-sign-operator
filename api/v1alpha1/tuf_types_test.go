package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
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

			It("key name", func() {
				invalidObject := generateTufObject("key-name")
				invalidObject.Spec.Keys[0].Name = "!@#$-name"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("body should match '^[-._a-zA-Z0-9]+$'")))
			})
		})

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
