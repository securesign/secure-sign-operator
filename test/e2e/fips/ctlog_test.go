//go:build fips

package fips

import (
	"crypto/elliptic"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/state"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign FIPS - ctlog test", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Reject non-FIPS ctlog key then accept FIPS-compliant key", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			invalidPub, invalidPriv, _, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, ctlog.CreateCustomCtlogSecret(namespace.Name, "my-invalid-ctlog-secret", map[string][]byte{
				"private": invalidPriv,
				"public":  invalidPub,
			}))).To(Succeed())
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret"))).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Ctlog.PrivateKeyRef = &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-invalid-ctlog-secret",
						},
						Key: "private",
					}
					v.Spec.Ctlog.PublicKeyRef = &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-invalid-ctlog-secret",
						},
						Key: "public",
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("CTlog reports ServerConfigAvailable False with SignerKey reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				ctlog := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ctlog).ToNot(BeNil())
				c := meta.FindStatusCondition(ctlog.Status.Conditions, ctlogactions.ConfigCondition)
				return c != nil && string(c.Status) == "False" &&
					c.Reason == ctlogactions.SignerKeyReason &&
					strings.Contains(strings.ToLower(c.Message), "waiting for ctlog signer key")
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update ctlog signer to use FIPS-compliant secret and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())

				s.Spec.Ctlog.PrivateKeyRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "my-ctlog-secret",
					},
					Key: "private",
				}
				s.Spec.Ctlog.PublicKeyRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "my-ctlog-secret",
					},
					Key: "public",
				}

				return cli.Update(ctx, s)
			}).Should(Succeed())
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})

	Describe("Reject non-FIPS ctlog custom server config then accept FIPS-compliant config", func() {
		var treeID int64

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
			ct := ctlog.Get(ctx, cli, namespace.Name, s.Name)
			Expect(ct).ToNot(BeNil())
			Expect(ct.Status.TreeID).ToNot(BeNil())
			treeID = *ct.Status.TreeID
		})

		BeforeAll(func(ctx SpecContext) {
			invalidPub, invalidPriv, _, err := fipsTest.GenerateECCertificatePEM(true, "pass", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			validPub, validPriv, validCert, err := fipsTest.GenerateECCertificatePEM(true, "pass", elliptic.P256())
			Expect(err).NotTo(HaveOccurred())

			trillianURL := fmt.Sprintf("trillian-logserver.%s.svc:8091", namespace.Name)

			invalidConfig, err := ctlogUtils.CreateCtlogConfig(
				trillianURL,
				treeID,
				[]ctlogUtils.RootCertificate{validCert},
				&ctlogUtils.KeyConfig{
					PrivateKey:     invalidPriv,
					PublicKey:      invalidPub,
					PrivateKeyPass: []byte("pass"),
				},
			)
			Expect(err).NotTo(HaveOccurred())

			validConfig, err := ctlogUtils.CreateCtlogConfig(
				trillianURL,
				treeID,
				[]ctlogUtils.RootCertificate{validCert},
				&ctlogUtils.KeyConfig{
					PrivateKey:     validPriv,
					PublicKey:      validPub,
					PrivateKeyPass: []byte("pass"),
				},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, ctlog.CreateCustomCtlogSecret(namespace.Name, "my-invalid-ctlog-config", invalidConfig))).To(Succeed())
			Expect(cli.Create(ctx, ctlog.CreateCustomCtlogSecret(namespace.Name, "my-ctlog-config", validConfig))).To(Succeed())
		})

		It("Update ctlog server config to non-FIPS secret and expect failure", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())
				s.Spec.Ctlog.ServerConfigRef = &v1alpha1.LocalObjectReference{
					Name: "my-invalid-ctlog-config",
				}
				return cli.Update(ctx, s)
			}).Should(Succeed())

			Eventually(func(g Gomega) bool {
				ct := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ct).ToNot(BeNil())
				c := meta.FindStatusCondition(ct.Status.Conditions, ctlogactions.ConfigCondition)
				return c != nil && string(c.Status) == "False" &&
					c.Reason == state.Failure.String() &&
					strings.Contains(strings.ToLower(c.Message), "invalid server config")
			}).WithContext(ctx).Should(BeTrue())
		})

		It("Update ctlog server config to FIPS-compliant secret and verify readiness", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: namespace.Name}, s)).To(Succeed())

				s.Spec.Ctlog.ServerConfigRef = &v1alpha1.LocalObjectReference{
					Name: "my-ctlog-config",
				}

				return cli.Update(ctx, s)
			}).Should(Succeed())
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
		})
	})

	Describe("Reject non-FIPS ctlog TLS", func() {

		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			_, invalidPriv, invalidCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
			Expect(err).NotTo(HaveOccurred())

			_, validPriv, validCert, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P256())
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, ctlog.CreateCustomCtlogSecret(namespace.Name, "my-invalid-ctlog-tls-secret", map[string][]byte{
				"tls.crt": invalidCert,
				"tls.key": invalidPriv,
			}))).To(Succeed())
			Expect(cli.Create(ctx, ctlog.CreateCustomCtlogSecret(namespace.Name, "my-ctlog-tls-secret", map[string][]byte{
				"tls.crt": validCert,
				"tls.key": validPriv,
			}))).To(Succeed())
		})

		BeforeAll(func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Ctlog.TLS = v1alpha1.TLS{
						CertRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-ctlog-tls-secret",
							},
							Key: "tls.crt",
						},
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-invalid-ctlog-tls-secret",
							},
							Key: "tls.key",
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("CTlog reports ServerTLS False with Failure reason", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				ct := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ct).ToNot(BeNil())
				c := meta.FindStatusCondition(ct.Status.Conditions, ctlogactions.TLSCondition)
				return c != nil && string(c.Status) == "False" &&
					c.Reason == state.Failure.String() &&
					strings.Contains(strings.ToLower(c.Message), "fips")
			}).WithContext(ctx).Should(BeTrue())
		})
	})
})
