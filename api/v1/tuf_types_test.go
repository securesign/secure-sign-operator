package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TUF", func() {

	Context("TufSpec", func() {
		It("can be created", func() {
			created := generateMinimalTuf("tuf-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Tuf{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalTuf("tuf-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Tuf{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Port = 8080
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalTuf("tuf-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateMinimalTuf("tuf-access-1")
				created.Spec.Ingress.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Tuf{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalTuf("tuf-access-2")
				created.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Tuf{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalTuf("tuf-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Tuf{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.Port).To(Equal(int32(80)))
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.RootKeySecretRef).To(Equal(&LocalObjectReference{Name: "tuf-root-keys"}))
			Expect(fetched.Spec.Ctlog).To(BeEmpty())
			Expect(fetched.Spec.Rekor).To(BeEmpty())
			Expect(fetched.Spec.Fulcio).To(BeEmpty())
			Expect(fetched.Spec.Tsa).To(BeNil())
			Expect(fetched.Spec.Ingress.Enabled).To(Equal(ptr.To(false)))
		})

		Context("is validated", func() {
			It("port is negative", func() {
				invalidObject := generateMinimalTuf("port-negative")
				invalidObject.Spec.Port = -20
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("should be greater than or equal to 1")))
			})

			It("port is bigger than 65535", func() {
				invalidObject := generateMinimalTuf("port-large")
				invalidObject.Spec.Port = 65536
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("should be less than or equal to 65535")))
			})

			It("more than one ctlog binding is rejected", func() {
				invalidObject := generateMinimalTuf("too-many-ctlog")
				invalidObject.Spec.Ctlog = []TrustRootBinding{{}, {}}
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Too many")))
			})

			When("service URL validation", func() {
				It("rejects ctlog without scheme", func() {
					obj := generateMinimalTuf("ctlog-no-scheme")
					obj.Spec.Ctlog = []TrustRootBinding{{ServiceReference: ServiceReference{URL: "ctlog:6963/path"}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), obj)).
						To(MatchError(ContainSubstring("url must follow the pattern scheme://host[:port]/path")))
				})

				It("rejects ctlog without path", func() {
					obj := generateMinimalTuf("ctlog-no-path")
					obj.Spec.Ctlog = []TrustRootBinding{{ServiceReference: ServiceReference{URL: "http://ctlog:6963"}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), obj)).
						To(MatchError(ContainSubstring("url must follow the pattern scheme://host[:port]/path")))
				})

				It("accepts ctlog with scheme and path", func() {
					obj := generateMinimalTuf("ctlog-valid")
					obj.Spec.Ctlog = []TrustRootBinding{{ServiceReference: ServiceReference{URL: "http://ctlog:6963/trusted-artifact-signer"}}}
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
				})

				It("rejects fulcio without scheme", func() {
					obj := generateMinimalTuf("fulcio-no-scheme")
					obj.Spec.Fulcio = []TrustRootBindingWithOIDC{{TrustRootBinding: TrustRootBinding{ServiceReference: ServiceReference{URL: "fulcio:5554"}}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), obj)).
						To(MatchError(ContainSubstring("url must follow the pattern scheme://host[:port]")))
				})

				It("accepts fulcio with scheme", func() {
					obj := generateMinimalTuf("fulcio-valid")
					obj.Spec.Fulcio = []TrustRootBindingWithOIDC{{TrustRootBinding: TrustRootBinding{ServiceReference: ServiceReference{URL: "http://fulcio:5554"}}}}
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
				})

				It("rejects rekor without scheme", func() {
					obj := generateMinimalTuf("rekor-no-scheme")
					obj.Spec.Rekor = []TrustRootBinding{{ServiceReference: ServiceReference{URL: "rekor:3000"}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), obj)).
						To(MatchError(ContainSubstring("url must follow the pattern scheme://host[:port]")))
				})

				It("accepts rekor with scheme", func() {
					obj := generateMinimalTuf("rekor-valid")
					obj.Spec.Rekor = []TrustRootBinding{{ServiceReference: ServiceReference{URL: "http://rekor:3000"}}}
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
				})

				It("rejects tsa without scheme", func() {
					obj := generateMinimalTuf("tsa-no-scheme")
					obj.Spec.Tsa = &[]TrustRootBinding{{ServiceReference: ServiceReference{URL: "tsa:3000"}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), obj)).
						To(MatchError(ContainSubstring("url must follow the pattern scheme://host[:port]")))
				})

				It("accepts tsa with scheme", func() {
					obj := generateMinimalTuf("tsa-valid")
					obj.Spec.Tsa = &[]TrustRootBinding{{ServiceReference: ServiceReference{URL: "http://tsa:3000"}}}
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
				})

				It("accepts a nil tsa (excluded from the trust root)", func() {
					obj := generateMinimalTuf("tsa-excluded")
					obj.Spec.Tsa = nil
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())

					fetched := &Tuf{}
					Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), fetched)).To(Succeed())
					Expect(fetched.Spec.Tsa).To(BeNil())
				})

				It("rejects fulcio oidc issuer without scheme", func() {
					obj := generateMinimalTuf("oidc-no-scheme")
					obj.Spec.Fulcio = []TrustRootBindingWithOIDC{{OIDCIssuers: []string{"accounts.google.com"}}}
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
				})

				It("accepts fulcio oidc issuer with scheme", func() {
					obj := generateMinimalTuf("oidc-valid")
					obj.Spec.Fulcio = []TrustRootBindingWithOIDC{{OIDCIssuers: []string{"https://accounts.google.com"}}}
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
				})
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalTuf("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive with ReadWriteOnce", func() {
					invalidObject := generateMinimalTuf("replicas-positive")
					invalidObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("more than 1 replica, pvc.accessModes must include 'ReadWriteMany'")))
				})

				It("one with ReadWriteOnce", func() {
					validObject := generateMinimalTuf("replicas-single")
					validObject.Spec.Replicas = ptr.To(int32(1))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive with ReadWriteMany", func() {
					validObject := generateMinimalTuf("replicas-positive-rwm")
					validObject.Spec.Replicas = ptr.To(int32(235469))
					validObject.Spec.Pvc.AccessModes = []PersistentVolumeAccessMode{
						PersistentVolumeAccessMode(v1.ReadWriteMany),
					}
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalTuf("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalTuf("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				tufInstance := Tuf{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tuf-full-manifest",
						Namespace: "default",
					},
					Spec: TufSpec{
						Port: 8181,
						Ingress: Ingress{
							Enabled: ptr.To(true),
							Host:    "hostname",
						},
						Rekor: []TrustRootBinding{
							{
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
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&tufInstance), fetchedtuf)).To(Succeed())
				Expect(fetchedtuf.Spec).To(Equal(tufInstance.Spec))
			})
		})
	})
})

func generateMinimalTuf(name string) *Tuf {
	return &Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}
