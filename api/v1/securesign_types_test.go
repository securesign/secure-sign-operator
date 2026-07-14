package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Securesign", func() {

	Context("SecuresignSpec", func() {
		It("can be created with minimal spec", func() {
			created := generateMinimalSecuresign("ss-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Securesign{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be deleted", func() {
			created := generateMinimalSecuresign("ss-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

	})

	Context("accepts empty sub-specs", func() {
		It("empty rekor", func() {
			obj := generateMinimalSecuresign("ss-empty-rekor")
			obj.Spec.Rekor = RekorSpec{}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("empty trillian", func() {
			obj := generateMinimalSecuresign("ss-empty-trillian")
			obj.Spec.Trillian = TrillianSpec{}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("empty tuf", func() {
			obj := generateMinimalSecuresign("ss-empty-tuf")
			obj.Spec.Tuf = TufSpec{}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("empty ctlog", func() {
			obj := generateMinimalSecuresign("ss-empty-ctlog")
			obj.Spec.Ctlog = CTlogSpec{}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("no tsa", func() {
			obj := generateMinimalSecuresign("ss-no-tsa")
			obj.Spec.TimestampAuthority = nil
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})
	})

	Context("replicas vs accessModes validation", func() {
		It("rejects tuf with replicas>1 and ReadWriteOnce", func() {
			obj := generateMinimalSecuresign("ss-tuf-rwx")
			obj.Spec.Tuf.Replicas = ptr.To(int32(2))
			Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
			Expect(k8sClient.Create(context.Background(), obj)).
				To(MatchError(ContainSubstring("pvc.accessModes must include 'ReadWriteMany'")))
		})

		It("accepts tuf with replicas>1 and ReadWriteMany", func() {
			obj := generateMinimalSecuresign("ss-tuf-rwm")
			obj.Spec.Tuf.Replicas = ptr.To(int32(2))
			obj.Spec.Tuf.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteMany"}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("rejects rekor with file:// attestations, replicas>1, and ReadWriteOnce", func() {
			obj := generateMinimalSecuresign("ss-rekor-rwx")
			obj.Spec.Rekor.Replicas = ptr.To(int32(2))
			Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
			Expect(k8sClient.Create(context.Background(), obj)).
				To(MatchError(ContainSubstring("ReadWriteMany")))
		})

		It("accepts rekor with replicas>1 and ReadWriteMany", func() {
			obj := generateMinimalSecuresign("ss-rekor-rwm")
			obj.Spec.Rekor.Replicas = ptr.To(int32(2))
			obj.Spec.Rekor.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteMany"}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})
	})

	Context("is validated", func() {
		It("requires OIDC issuers", func() {
			obj := &Securesign{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ss-no-oidc",
					Namespace: "default",
				},
				Spec: SecuresignSpec{},
			}
			Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
			Expect(k8sClient.Create(context.Background(), obj)).
				To(MatchError(ContainSubstring("At least one OIDC issuer or meta issuer must be configured")))
		})

		It("accepts MetaIssuers instead of OIDCIssuers", func() {
			obj := generateMinimalSecuresign("ss-meta-issuer")
			obj.Spec.Fulcio.Config.OIDCIssuers = nil
			obj.Spec.Fulcio.Config.MetaIssuers = []OIDCIssuer{
				{ClientID: "client", Type: "email", Issuer: "url"},
			}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
		})

		It("rejects empty OIDCIssuers and MetaIssuers", func() {
			obj := generateMinimalSecuresign("ss-empty-issuers")
			obj.Spec.Fulcio.Config.OIDCIssuers = []OIDCIssuer{}
			obj.Spec.Fulcio.Config.MetaIssuers = []OIDCIssuer{}
			Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), obj))).To(BeTrue())
			Expect(k8sClient.Create(context.Background(), obj)).
				To(MatchError(ContainSubstring("At least one OIDC issuer or meta issuer must be configured")))
		})
	})

	Context("partial sub-specs with user overrides", func() {
		It("rekor with custom replicas", func() {
			obj := generateMinimalSecuresign("ss-rekor-replicas")
			obj.Spec.Rekor.Replicas = ptr.To(int32(3))
			obj.Spec.Rekor.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteMany"}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())

			fetched := &Securesign{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Spec.Rekor.Replicas).To(Equal(ptr.To(int32(3))))
		})

		It("tuf with custom port", func() {
			obj := generateMinimalSecuresign("ss-tuf-port")
			obj.Spec.Tuf.Port = 8080
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())

			fetched := &Securesign{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Spec.Tuf.Port).To(Equal(int32(8080)))
		})

		It("fulcio with certificate", func() {
			obj := generateMinimalSecuresign("ss-fulcio-cert")
			obj.Spec.Fulcio.Certificate = FulcioCert{
				OrganizationName: "test-org",
			}
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())

			fetched := &Securesign{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Spec.Fulcio.Certificate.OrganizationName).To(Equal("test-org"))
		})
	})
})

func generateMinimalSecuresign(name string) *Securesign {
	return &Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: SecuresignSpec{
			Fulcio: FulcioSpec{
				Config: FulcioConfig{
					OIDCIssuers: []OIDCIssuer{
						{
							ClientID:  "client",
							Type:      "email",
							IssuerURL: "url",
							Issuer:    "url",
						},
					},
				},
				Certificate: FulcioCert{
					OrganizationName: "org",
				},
			},
		},
	}
}
