//go:build fips

package fips

import (
	"crypto/elliptic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - rekor signer test", Ordered, func() {

	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithProvidedCerts(),
			func(v *v1alpha1.Securesign) {
				v.Spec.Tuf.Keys = []v1alpha1.TufKey{
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
					{
						Name: "rekor.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-tuf-rekor-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-ctlog-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "certificateChain",
						},
					},
				}
			},
		)
	})

	Describe("Reject non-FIPS Rekor private key then accept FIPS-compliant key", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret"))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret"))).To(Succeed())
			Expect(cli.Create(ctx, createCustomRekorSecret(namespace.Name, "my-rekor-secret"))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-tuf-rekor-secret"))).To(Succeed())
			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "test-tsa-secret"))).To(Succeed())
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
							Name: "my-tuf-rekor-secret",
						},
						Key: "private",
					},
				}

				return cli.Update(ctx, s)
			}).Should(Succeed())
			rekor.Verify(ctx, cli, namespace.Name, s.Name, true)
		})

	})

})

func createCustomRekorSecret(ns string, name string) *v1.Secret {
	private := fipsTest.GenerateECPrivateKeyPEM(&testing.T{}, elliptic.P224())
	public := fipsTest.GenerateECPublicKeyPEM(&testing.T{}, elliptic.P224())
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}
