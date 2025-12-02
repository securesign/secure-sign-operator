//go:build fips

package fips

import (
	"crypto/elliptic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	tufhelpers "github.com/securesign/operator/test/e2e/support/tas/tuf"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - tuf test", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Reject non-FIPS tuf key then accept FIPS-compliant key", func() {
		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			invalidPub, _, _, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			validPub, _, validCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-invalid-tuf-keys", Namespace: namespace.Name}, Data: map[string][]byte{
				"rekor.pub":         invalidPub,
				"ctfe.pub":          validPub,
				"fulcio_v1.crt.pem": validCert,
				"tsa.certchain.pem": validCert,
			}})).To(Succeed())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-tuf-keys", Namespace: namespace.Name}, Data: map[string][]byte{
				"rekor.pub":         validPub,
				"ctfe.pub":          validPub,
				"fulcio_v1.crt.pem": validCert,
				"tsa.certchain.pem": validCert,
			}})).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "key-test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Tuf.Keys = []v1alpha1.TufKey{
						{
							Name: "rekor.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-keys",
								},
								Key: "rekor.pub",
							},
						},
						{
							Name: "ctfe.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-keys",
								},
								Key: "ctfe.pub",
							},
						},
						{
							Name: "fulcio_v1.crt.pem",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-keys",
								},
								Key: "fulcio_v1.crt.pem",
							},
						},
						{
							Name: "tsa.certchain.pem",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-keys",
								},
								Key: "tsa.certchain.pem",
							},
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Tuf reports key condition False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				tf := tufhelpers.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(tf).ToNot(BeNil())
				c := meta.FindStatusCondition(tf.Status.Conditions, "rekor.pub")
				return c != nil && string(c.Status) == "False" && c.Reason == constants.Failure
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update tuf to use FIPS-compliant keys and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())
				s.Spec.Tuf.Keys = []v1alpha1.TufKey{
					{
						Name: "rekor.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-keys",
							},
							Key: "rekor.pub",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-keys",
							},
							Key: "ctfe.pub",
						},
					},
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-keys",
							},
							Key: "fulcio_v1.crt.pem",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-keys",
							},
							Key: "tsa.certchain.pem",
						},
					},
				}
				return cli.Update(ctx, s)
			}).WithContext(ctx).Should(Succeed())

			tufhelpers.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})

	Describe("Reject non-FIPS tuf cert then accept FIPS-compliant cert", func() {
		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			_, _, invalidCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			validPub, _, validCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-invalid-tuf-cert", Namespace: namespace.Name}, Data: map[string][]byte{
				"rekor.pub":         validPub,
				"ctfe.pub":          validPub,
				"fulcio_v1.crt.pem": invalidCert,
				"tsa.certchain.pem": validCert,
			}})).To(Succeed())

			Expect(cli.Create(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-tuf-cert", Namespace: namespace.Name}, Data: map[string][]byte{
				"rekor.pub":         validPub,
				"ctfe.pub":          validPub,
				"fulcio_v1.crt.pem": validCert,
				"tsa.certchain.pem": validCert,
			}})).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "cert-test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Tuf.Keys = []v1alpha1.TufKey{
						{
							Name: "rekor.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-cert",
								},
								Key: "rekor.pub",
							},
						},
						{
							Name: "ctfe.pub",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-cert",
								},
								Key: "ctfe.pub",
							},
						},
						{
							Name: "fulcio_v1.crt.pem",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-cert",
								},
								Key: "fulcio_v1.crt.pem",
							},
						},
						{
							Name: "tsa.certchain.pem",
							SecretRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "my-invalid-tuf-cert",
								},
								Key: "tsa.certchain.pem",
							},
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Tuf reports key condition False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				tf := tufhelpers.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(tf).ToNot(BeNil())
				c := meta.FindStatusCondition(tf.Status.Conditions, "fulcio_v1.crt.pem")
				return c != nil && string(c.Status) == "False" && c.Reason == constants.Failure
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update tuf to use FIPS-compliant keys and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())
				s.Spec.Tuf.Keys = []v1alpha1.TufKey{
					{
						Name: "rekor.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-cert",
							},
							Key: "rekor.pub",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-cert",
							},
							Key: "ctfe.pub",
						},
					},
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-cert",
							},
							Key: "fulcio_v1.crt.pem",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-cert",
							},
							Key: "tsa.certchain.pem",
						},
					},
				}
				return cli.Update(ctx, s)
			}).WithContext(ctx).Should(Succeed())

			tufhelpers.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})
})
