//go:build fips

package fips

import (
	"crypto/elliptic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	tsaactions "github.com/securesign/operator/internal/controller/tsa/actions"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - TSA Cert chain", Ordered, func() {

	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Reject non-FIPS TSA Cert chain and key then accept FIPS-compliant key", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			_, invalidPriv, invalidCert, err := fipsTest.GenerateECCertificatePEM(true, "pass", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, tsa.CreateCustomTsaSecret(namespace.Name, "my-invalid-tsa-secret", map[string][]byte{
				"leafPrivateKey":         invalidPriv,
				"leafPrivateKeyPassword": []byte("pass"),
				"certificateChain":       invalidCert,
			}))).To(Succeed())

			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "my-tsa-secret"))).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.TimestampAuthority.Signer = v1alpha1.TimestampAuthoritySigner{
						CertificateChain: v1alpha1.CertificateChain{
							CertificateChainRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tsa-secret",
								},
								Key: "certificateChain",
							},
						},
						File: &v1alpha1.File{
							PrivateKeyRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tsa-secret",
								},
								Key: "leafPrivateKey",
							},
							PasswordRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tsa-secret",
								},
								Key: "leafPrivateKeyPassword",
							},
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("TSA reports TSASignerCondition False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				t := tsa.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(t).ToNot(BeNil())
				c := meta.FindStatusCondition(t.Status.Conditions, tsaactions.TSASignerCondition)
				return c != nil && string(c.Status) == "False" && c.Reason == constants.Failure
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update TSA secret to FIPS-compliant EC P256/P384 and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())

				s.Spec.TimestampAuthority.Signer = v1alpha1.TimestampAuthoritySigner{
					CertificateChain: v1alpha1.CertificateChain{
						CertificateChainRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tsa-secret",
							},
							Key: "certificateChain",
						},
					},
					File: &v1alpha1.File{
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tsa-secret",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tsa-secret",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}
				return cli.Update(ctx, s)
			}).Should(Succeed())
			tsa.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})
})
