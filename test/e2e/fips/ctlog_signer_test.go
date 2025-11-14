//go:build fips

package fips

import (
	"crypto/elliptic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
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

var _ = Describe("Securesign FIPS - ctlog signer test", Ordered, func() {
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
								Name: "my-rekor-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-ctlog-tuf-secret",
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

	Describe("Reject non-FIPS ctlog key then accept FIPS-compliant key", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, createCtlogSecret(namespace.Name, "my-ctlog-secret", elliptic.P224()))).To(Succeed())
			Expect(cli.Create(ctx, createCtlogSecret(namespace.Name, "my-ctlog-tuf-secret", elliptic.P256()))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret"))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret"))).To(Succeed())
			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "test-tsa-secret"))).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("CTlog reports ServerConfigAvailable False with SignerKey reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				ctlog := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ctlog).ToNot(BeNil())
				c := meta.FindStatusCondition(ctlog.Status.Conditions, ctlogactions.ConfigCondition)
				return c != nil && string(c.Status) == "False" && c.Reason == ctlogactions.SignerKeyReason
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update ctlog secret to FIPS-compliant EC P256 and verify readiness", func(ctx SpecContext) {
			sec := &v1.Secret{}
			Expect(cli.Get(ctx, types.NamespacedName{Name: "my-ctlog-secret", Namespace: namespace.Name}, sec)).To(Succeed())
			priv := fipsTest.GenerateECPrivateKeyPEM(&testing.T{}, elliptic.P256())
			pub := fipsTest.GenerateECPublicKeyPEM(&testing.T{}, elliptic.P256())

			sec.Data["private"], sec.Data["public"] = priv, pub
			Expect(cli.Update(ctx, sec)).To(Succeed())
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})
})

func createCtlogSecret(ns, name string, curve elliptic.Curve) *v1.Secret {
	priv := fipsTest.GenerateECPrivateKeyPEM(&testing.T{}, curve)
	pub := fipsTest.GenerateECPublicKeyPEM(&testing.T{}, curve)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": priv,
			"public":  pub,
		},
	}
}
