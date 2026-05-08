//go:build integration

package update

import (
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/test/e2e/support/steps"

	"github.com/securesign/operator/test/e2e/support/tas"

	fulcioAction "github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var _ = Describe("PKCS#11 Fulcio update", Ordered, func() {
	SetDefaultEventuallyTimeout(time.Duration(6) * time.Minute)
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithPKCS11Certs(),
			securesign.WithManagedDatabase(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
			securesign.WithoutSearchUI(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with PKCS#11 certificates", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})
	})

	Describe("Changes OIDC configuration with PKCS#11 baseline", func() {
		var fulcioGeneration int64

		It("stored current deployment observed generations", func(ctx SpecContext) {
			fulcioGeneration = getDeploymentGeneration(ctx, cli,
				types.NamespacedName{Namespace: namespace.Name, Name: fulcioAction.DeploymentName},
			)
			Expect(fulcioGeneration).Should(BeNumerically(">", 0))
		})

		It("adds new OIDCIssuers", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), s)).To(Succeed())
				s.Spec.Fulcio.Config.OIDCIssuers = append(s.Spec.Fulcio.Config.OIDCIssuers, v1alpha1.OIDCIssuer{
					ClientID:  "fake",
					IssuerURL: "fake",
					Issuer:    "fake",
					Type:      "email",
				})
				return cli.Update(ctx, s)
			}).WithTimeout(1 * time.Second).Should(Succeed())
		})

		It("has status Ready", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				ctl := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ctl).NotTo(BeNil())
				return meta.FindStatusCondition(ctl.Status.Conditions, constants.ReadyCondition).Reason
			}).Should(Equal(state.Ready.String()))
		})

		It("updated Fulcio deployment", func(ctx SpecContext) {
			Eventually(func() int64 {
				return getDeploymentGeneration(ctx, cli, types.NamespacedName{Namespace: namespace.Name, Name: fulcioAction.DeploymentName})
			}).Should(BeNumerically(">", fulcioGeneration))
		})

		It("verify Fulcio is ready", func(ctx SpecContext) {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("verify new OIDC configuration in ConfigMap", func(ctx SpecContext) {
			var f *v1alpha1.Fulcio
			var fulcioPod *v1.Pod
			Eventually(func(g Gomega) {
				f = fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				fulcioPod = fulcio.GetServerPod(ctx, cli, namespace.Name)()
				g.Expect(fulcioPod).NotTo(BeNil())
			}).Should(Succeed())

			var configVolName string
			for _, vol := range fulcioPod.Spec.Volumes {
				if vol.ConfigMap != nil && vol.ConfigMap.Name == f.Status.ServerConfigRef.Name {
					configVolName = vol.Name
					break
				}
			}
			Expect(configVolName).NotTo(BeEmpty(), "fulcio-config volume should reference the server config ConfigMap")

			cm := &v1.ConfigMap{}
			Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: f.Status.ServerConfigRef.Name}, cm)).To(Succeed())
			config := &fulcioAction.FulcioMapConfig{}
			Expect(yaml.Unmarshal([]byte(cm.Data["config.yaml"]), config)).To(Succeed())
			Expect(config.OIDCIssuers).To(HaveKey("fake"))
		})

		It("Fulcio still uses PKCS#11 CA args", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			var fulcioContainer *v1.Container
			for i := range server.Spec.Containers {
				if server.Spec.Containers[i].Name == "fulcio-server" {
					fulcioContainer = &server.Spec.Containers[i]
					break
				}
			}
			Expect(fulcioContainer).NotTo(BeNil())
			Expect(fulcioContainer.Args).To(ContainElements("--ca=pkcs11ca"))
			Expect(fulcioContainer.Args).NotTo(ContainElements("--ca=fileca"))
		})

		It("verify by cosign", NodeTimeout(8*time.Minute), func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
