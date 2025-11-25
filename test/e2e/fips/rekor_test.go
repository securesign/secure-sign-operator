//go:build fips

package fips

import (
	"crypto/elliptic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - rekor test", Ordered, func() {

	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Reject non-FIPS rekor key then accept FIPS-compliant key", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			invalidPub, invalidPriv, _, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, rekor.CreateCustomRekorSecret(namespace.Name, "my-invalid-rekor-secret", map[string][]byte{
				"private": invalidPriv,
				"public":  invalidPub,
			}))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret"))).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Rekor.Signer = v1alpha1.RekorSigner{
						KMS: "secret",
						KeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-rekor-secret",
							},
							Key: "private",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Rekor reports SignerAvailable False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				rk := rekor.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(rk).ToNot(BeNil())
				c := meta.FindStatusCondition(rk.Status.Conditions, rekoractions.SignerCondition)
				return c != nil && string(c.Status) == "False" && c.Reason == constants.Failure
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update Rekor signer to use FIPS-compliant secret and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())

				s.Spec.Rekor.Signer = v1alpha1.RekorSigner{
					KMS: "secret",
					KeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-rekor-secret",
						},
						Key: "private",
					},
				}

				return cli.Update(ctx, s)
			}).Should(Succeed())
			rekor.Verify(ctx, cli, namespace.Name, s.Name, true)
		})

	})

	Describe("Reject non-FIPS rekor redis TLS", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			_, invalidPriv, invalidCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			_, validPriv, validCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-invalid-rekor-redis-tls-secret", Namespace: namespace.Name},
				Data: map[string][]byte{
					"tls.crt": invalidCert,
					"tls.key": invalidPriv,
				}})).To(Succeed())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-rekor-redis-tls-secret", Namespace: namespace.Name},
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": validPriv,
				},
			})).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Rekor.SearchIndex.TLS = v1alpha1.TLS{
						CertRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-rekor-redis-tls-secret",
							},
							Key: "tls.crt",
						},
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-rekor-redis-tls-secret",
							},
							Key: "tls.key",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Rekor reports RedisAvailable False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				rk := rekor.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(rk).ToNot(BeNil())
				c := meta.FindStatusCondition(rk.Status.Conditions, rekoractions.RedisCondition)
				return c != nil && string(c.Status) == "False" && c.Reason == constants.Failure
			}).WithContext(ctx).Should(BeTrue())
		})
	})
})
