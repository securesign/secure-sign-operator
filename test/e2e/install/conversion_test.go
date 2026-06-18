//go:build integration

package install

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Conversion webhook", Ordered, func() {
	SetDefaultEventuallyTimeout(5 * time.Minute)
	cli, _ := support.CreateClient()

	var (
		namespace   *v1.Namespace
		s           *rhtasv1.Securesign
		fipsEnabled bool
	)

	BeforeAll(steps.DetectAndConfigureFIPS(cli, func(enabled bool) {
		fipsEnabled = enabled
	}))

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		if fipsEnabled {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "conversion-test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
		)
		Expect(cli.Create(ctx, s)).To(Succeed())
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	Context("Securesign", func() {
		It("should preserve spec and status across versions", func(ctx SpecContext) {
			v1Obj := &rhtasv1.Securesign{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.Securesign{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Spec.Fulcio.Config.OIDCIssuers).ToNot(BeEmpty())
			Expect(v1Obj.Spec.Fulcio.Config.OIDCIssuers[0].Issuer).To(Equal(v1alpha1Obj.Spec.Fulcio.Config.OIDCIssuers[0].Issuer))
			Expect(v1Obj.Spec.TimestampAuthority).ToNot(BeNil())
			Expect(v1Obj.Spec.TimestampAuthority.Signer.CertificateChain.RootCA.OrganizationName).
				To(Equal(v1alpha1Obj.Spec.TimestampAuthority.Signer.CertificateChain.RootCA.OrganizationName))
			Expect(v1Obj.Status.RekorStatus.Url).ToNot(BeEmpty())
			Expect(v1Obj.Status.RekorStatus.Url).To(Equal(v1alpha1Obj.Status.RekorStatus.Url))
		})
	})

	Context("Fulcio", func() {
		It("should preserve certificate and OIDC config", func(ctx SpecContext) {
			v1Obj := &rhtasv1.Fulcio{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.Fulcio{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Spec.Certificate.OrganizationName).To(Equal(v1alpha1Obj.Spec.Certificate.OrganizationName))
			Expect(v1Obj.Spec.Certificate.OrganizationEmail).To(Equal(v1alpha1Obj.Spec.Certificate.OrganizationEmail))
			Expect(v1Obj.Spec.Certificate.CommonName).To(Equal(v1alpha1Obj.Spec.Certificate.CommonName))

			Expect(v1Obj.Spec.Config.OIDCIssuers).To(HaveLen(len(v1alpha1Obj.Spec.Config.OIDCIssuers)))
			Expect(v1Obj.Spec.Config.OIDCIssuers[0].Issuer).To(Equal(v1alpha1Obj.Spec.Config.OIDCIssuers[0].Issuer))
			Expect(v1Obj.Spec.Config.OIDCIssuers[0].ClientID).To(Equal(v1alpha1Obj.Spec.Config.OIDCIssuers[0].ClientID))
			Expect(v1Obj.Spec.Config.OIDCIssuers[0].Type).To(Equal(v1alpha1Obj.Spec.Config.OIDCIssuers[0].Type))

			By("verifying auto-generated key references in status")
			Expect(v1Obj.Status.Certificate.PrivateKeyRef).ToNot(BeNil())
			Expect(v1Obj.Status.Certificate.PrivateKeyRef.Name).To(Equal(v1alpha1Obj.Status.Certificate.PrivateKeyRef.Name))
			Expect(v1Obj.Status.Certificate.PrivateKeyRef.Key).To(Equal(v1alpha1Obj.Status.Certificate.PrivateKeyRef.Key))
			Expect(v1Obj.Status.Certificate.CARef).ToNot(BeNil())
			Expect(v1Obj.Status.Certificate.CARef.Name).To(Equal(v1alpha1Obj.Status.Certificate.CARef.Name))
		})
	})

	Context("Rekor", func() {
		It("should preserve TreeID and signer config", func(ctx SpecContext) {
			v1Obj := &rhtasv1.Rekor{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.Rekor{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Status.TreeID).ToNot(BeNil())
			Expect(v1Obj.Status.TreeID).To(Equal(v1alpha1Obj.Status.TreeID))

			Expect(v1Obj.Status.Signer.KeyRef).ToNot(BeNil())
			Expect(v1Obj.Status.Signer.KeyRef.Name).To(Equal(v1alpha1Obj.Status.Signer.KeyRef.Name))
			Expect(v1Obj.Status.Signer.KeyRef.Key).To(Equal(v1alpha1Obj.Status.Signer.KeyRef.Key))

			Expect(v1Obj.Status.Url).ToNot(BeEmpty())
			Expect(v1Obj.Status.Url).To(Equal(v1alpha1Obj.Status.Url))
		})
	})

	Context("CTlog", func() {
		It("should preserve TreeID and key references", func(ctx SpecContext) {
			v1Obj := &rhtasv1.CTlog{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.CTlog{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Status.TreeID).ToNot(BeNil())
			Expect(v1Obj.Status.TreeID).To(Equal(v1alpha1Obj.Status.TreeID))

			Expect(v1Obj.Status.PrivateKeyRef).ToNot(BeNil())
			Expect(v1Obj.Status.PrivateKeyRef.Name).To(Equal(v1alpha1Obj.Status.PrivateKeyRef.Name))
			Expect(v1Obj.Status.PrivateKeyRef.Key).To(Equal(v1alpha1Obj.Status.PrivateKeyRef.Key))

			Expect(v1Obj.Status.RootCertificates).To(HaveLen(len(v1alpha1Obj.Status.RootCertificates)))
		})
	})

	Context("TSA", func() {
		It("should preserve signer certificate chain and NTP config", func(ctx SpecContext) {
			v1Obj := &rhtasv1.TimestampAuthority{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.TimestampAuthority{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			By("verifying certificate chain")
			Expect(v1Obj.Spec.Signer.CertificateChain.RootCA).ToNot(BeNil())
			Expect(v1Obj.Spec.Signer.CertificateChain.RootCA.OrganizationName).
				To(Equal(v1alpha1Obj.Spec.Signer.CertificateChain.RootCA.OrganizationName))

			Expect(v1Obj.Spec.Signer.CertificateChain.IntermediateCA).To(HaveLen(len(v1alpha1Obj.Spec.Signer.CertificateChain.IntermediateCA)))
			Expect(v1Obj.Spec.Signer.CertificateChain.IntermediateCA[0].OrganizationName).
				To(Equal(v1alpha1Obj.Spec.Signer.CertificateChain.IntermediateCA[0].OrganizationName))

			Expect(v1Obj.Spec.Signer.CertificateChain.LeafCA).ToNot(BeNil())
			Expect(v1Obj.Spec.Signer.CertificateChain.LeafCA.OrganizationName).
				To(Equal(v1alpha1Obj.Spec.Signer.CertificateChain.LeafCA.OrganizationName))

			By("verifying NTP monitoring config")
			Expect(v1Obj.Spec.NTPMonitoring.Enabled).To(Equal(v1alpha1Obj.Spec.NTPMonitoring.Enabled))
			Expect(v1Obj.Spec.NTPMonitoring.Config).ToNot(BeNil())
			Expect(v1Obj.Spec.NTPMonitoring.Config.Servers).To(Equal(v1alpha1Obj.Spec.NTPMonitoring.Config.Servers))
			Expect(v1Obj.Spec.NTPMonitoring.Config.RequestAttempts).To(Equal(v1alpha1Obj.Spec.NTPMonitoring.Config.RequestAttempts))
		})
	})

	Context("Tuf", func() {
		It("should preserve keys and port", func(ctx SpecContext) {
			v1Obj := &rhtasv1.Tuf{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.Tuf{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Status.Keys).To(HaveLen(len(v1alpha1Obj.Status.Keys)))
			for i := range v1Obj.Status.Keys {
				Expect(v1Obj.Status.Keys[i].Name).To(Equal(v1alpha1Obj.Status.Keys[i].Name))
			}
			Expect(v1Obj.Spec.Port).To(Equal(v1alpha1Obj.Spec.Port))
		})
	})

	Context("Trillian", func() {
		It("should preserve database config", func(ctx SpecContext) {
			v1Obj := &rhtasv1.Trillian{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Obj)).To(Succeed())

			v1alpha1Obj := &v1alpha1.Trillian{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1alpha1Obj)).To(Succeed())

			Expect(v1Obj.Spec.Db.Create).ToNot(BeNil())
			Expect(*v1Obj.Spec.Db.Create).To(Equal(*v1alpha1Obj.Spec.Db.Create))
			Expect(v1Obj.Spec.Db.Provider).To(Equal(v1alpha1Obj.Spec.Db.Provider))
		})
	})
})

func nsName(ns, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: ns, Name: name}
}
