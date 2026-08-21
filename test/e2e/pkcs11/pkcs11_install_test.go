//go:build pkcs11

package pkcs11

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	pkcs11support "github.com/securesign/operator/test/e2e/support/tas/pkcs11"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

var _ = Describe("Securesign install with PKCS#11/HSM signer", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *rhtasv1.Securesign
	var fipsEnabled bool

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
		// Deploy SoftHSM infrastructure, run key ceremonies, and create prerequisite secrets.
		Expect(pkcs11support.CreatePrerequisites(ctx, cli, namespace.Name)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
			securesign.WithPKCS11Signer(namespace.Name),
		)
		Expect(cli.Create(ctx, s)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("PKCS#11 install", func() {
		It("all components reach Ready", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
		})

		It("Fulcio runs in PKCS#11 mode", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) *v1.Pod {
				return fulcio.GetServerPod(ctx, cli, namespace.Name)()
			}).WithContext(ctx).ShouldNot(BeNil())

			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			// Verify init container is present.
			Expect(server.Spec.InitContainers).To(
				ContainElement(
					WithTransform(func(c v1.Container) string {
						return c.Name
					}, Equal("hsm-lib-export")),
				))

			// Verify PKCS#11 config volume is mounted.
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(vol v1.Volume) string {
						return vol.Name
					}, Equal(fulcioactions.PKCS11ConfigVolumeName)),
				))
		})

		It("CTLog runs with PKCS#11 module", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) *v1.Pod {
				return ctlog.GetServerPod(ctx, cli, namespace.Name)
			}).WithContext(ctx).ShouldNot(BeNil())

			server := ctlog.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).NotTo(BeNil())

			// Verify init container is present.
			Expect(server.Spec.InitContainers).To(
				ContainElement(
					WithTransform(func(c v1.Container) string {
						return c.Name
					}, Equal("hsm-lib-export")),
				))

			// Verify HSM token volume is mounted.
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(vol v1.Volume) string {
						return vol.Name
					}, Equal("hsm-tokens")),
				))
		})

		It("PKCS#11 conditions are set", func(ctx SpecContext) {
			// Verify FulcioPKCS11ConfigAvailable condition.
			Eventually(func(ctx context.Context) bool {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				if f == nil {
					return false
				}
				return meta.IsStatusConditionTrue(f.GetConditions(), fulcioactions.PKCS11Condition)
			}).WithContext(ctx).Should(BeTrue())

			// Verify PKCS11ConfigAvailable condition on CTLog.
			Eventually(func(ctx context.Context) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				if c == nil {
					return false
				}
				return meta.IsStatusConditionTrue(c.GetConditions(), ctlogactions.PKCS11Condition)
			}).WithContext(ctx).Should(BeTrue())
		})

		It("SecureSign CR reaches Ready state", func(ctx SpecContext) {
			securesign.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("Fulcio CR shows certificate chain in status", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) bool {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				if f == nil {
					return false
				}
				return condition.IsReady(f)
			}).WithContext(ctx).Should(BeTrue())
		})

		It("cosign sign and verify", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName,
				s.Status.TufStatus.Url,
				s.Status.FulcioStatus.Url,
				s.Status.RekorStatus.Url,
				s.Status.TSAStatus.Url,
			)
		})
	})
})
