//go:build fips

package fips

import (
	"crypto/elliptic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	fulciohelpers "github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - fulcio cert test", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Reject non-FIPS fulcio key and cert then accept FIPS-compliant key and cert", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			_, invalidPriv, invalidCert, err := fipsTest.GenerateECCertificatePEM(true, "pass", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, fulciohelpers.CreateCustomFulcioSecret(namespace.Name, "my-invalid-fulcio-secret",
				map[string][]byte{
					"password": []byte("pass"),
					"private":  invalidPriv,
					"cert":     invalidCert,
				}))).To(Succeed())

			Expect(cli.Create(ctx, fulciohelpers.CreateSecret(namespace.Name, "my-fulcio-secret"))).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Fulcio.Certificate = v1alpha1.FulcioCert{
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-fulcio-secret",
							},
							Key: "private",
						},
						PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-fulcio-secret",
							},
							Key: "password",
						},
						CARef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-fulcio-secret",
							},
							Key: "cert",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Fulcio reports FulcioCertAvailable with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				f := fulciohelpers.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).ToNot(BeNil())
				c := meta.FindStatusCondition(f.Status.Conditions, fulcioactions.CertCondition)
				g.Expect(c).ToNot(BeNil())
				return c.Reason
			}).WithContext(ctx).Should(Equal(constants.Failure))
		})

		It("Update fulcio to use FIPS-compliant secret and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())

				s.Spec.Fulcio.Certificate = v1alpha1.FulcioCert{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "private",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "password",
					},
					CARef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "cert",
					},
				}
				return cli.Update(ctx, s)
			}).Should(Succeed())

			fulciohelpers.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})

})
