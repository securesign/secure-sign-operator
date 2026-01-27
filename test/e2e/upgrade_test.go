//go:build upgrade

package e2e

import (
	"fmt"
	"os"
	"time"

	"github.com/blang/semver/v4"
	"github.com/onsi/ginkgo/v2/dsl/core"
	tasv1alpha "github.com/securesign/operator/api/v1alpha1"
	ctl "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioAction "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekorAction "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianAction "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaAction "github.com/securesign/operator/internal/controller/tsa/actions"
	tufAction "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/kubernetes/olm"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v13 "k8s.io/api/apps/v1"
	rbacV1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Operator upgrade", Ordered, func() {
	gomega.SetDefaultEventuallyTimeout(5 * time.Minute)
	cli, _ := support.CreateClient()

	var (
		namespace                              *v1.Namespace
		baseCatalogImage, targetedCatalogImage string
		baseVersion                            string
		securesignDeployment                   *tasv1alpha.Securesign
		rrekor                                 *tasv1alpha.Rekor
		prevImageName, newImageName            string
		err                                    error
		extension                              olm.Extension
		catalog                                olm.ExtensionSource
	)

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))
	BeforeAll(func(ctx SpecContext) {
		DeferCleanup(func(ctx SpecContext) {
			if extension != nil {
				_ = cli.Delete(ctx, extension.Unwrap())
			}
			if catalog != nil {
				_ = cli.Delete(ctx, catalog.Unwrap())
			}
			if _, ok := os.LookupEnv("OLM_V1"); ok {
				_ = cli.Delete(ctx, &rbacV1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-installer-%s", "rhtas-operator", namespace.Name),
					},
				})
			}
		})
	})

	JustAfterEach(func(ctx SpecContext) {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			core.GinkgoWriter.Println("----------------------- Dumping operator resources -----------------------")
			core.GinkgoWriter.Println("\n\nCatalog:")
			core.GinkgoWriter.Printf("%s ready: %v\n", catalog.GetName(), catalog.IsReady(ctx, cli))
			core.GinkgoWriter.Println("\n\nExtension:")
			core.GinkgoWriter.Printf("%s version: %s ready: %v\n", extension.GetName(), extension.GetVersion(ctx, cli), extension.IsReady(ctx, cli))
		}
	})

	BeforeAll(func(ctx SpecContext) {
		baseCatalogImage = os.Getenv("TEST_BASE_CATALOG")
		targetedCatalogImage = os.Getenv("TEST_TARGET_CATALOG")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	BeforeAll(func(ctx SpecContext) {
		prevImageName = support.PrepareImage(ctx)
		newImageName = support.PrepareImage(ctx)
	})

	It("Install TAS", func(ctx SpecContext) {
		if _, ok := os.LookupEnv("OLM_V1"); ok {
			extension, catalog, err = olm.OlmV1Installer(ctx, cli, baseCatalogImage, namespace.Name, "rhtas-operator")
		} else {
			extension, catalog, err = olm.OlmInstaller(ctx, cli, baseCatalogImage, namespace.Name, "rhtas-operator", "stable", testSupportKubernetes.IsRemoteClusterOpenshift())
		}
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			return catalog.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(extension), extension.Unwrap())).To(gomega.Succeed())
			return extension.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(extension.GetVersion).WithArguments(ctx, cli).Should(gomega.Not(gomega.BeEmpty()))

		baseVersion = extension.GetVersion(ctx, cli)

		gomega.Eventually(func(g gomega.Gomega) []v13.Deployment {
			list := &v13.DeploymentList{}
			g.Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppPartOf: "rhtas-operator"})).To(gomega.Succeed())
			return list.Items
		}).Should(gomega.And(gomega.HaveLen(1), gomega.WithTransform(func(items []v13.Deployment) int32 {
			return items[0].Status.AvailableReplicas
		}, gomega.BeNumerically(">=", 1))))
	})

	It("Install securesign", func(ctx SpecContext) {
		securesignDeployment = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithSearchUI(),
			securesign.WithMonitoring(),
			func(v *tasv1alpha.Securesign) {
				v.Spec.Trillian.Db.Pvc.Retain = nil
			},
			func(v *tasv1alpha.Securesign) {
				if v.Annotations == nil {
					v.Annotations = map[string]string{}
				}

				if testSupportKubernetes.IsRemoteClusterOpenshift() {
					v.Annotations["rhtas.redhat.com/metrics"] = "true"
				} else {
					v.Annotations["rhtas.redhat.com/metrics"] = "false"
				}
			},
		)
		gomega.Expect(cli.Create(ctx, securesignDeployment)).To(gomega.Succeed())

		tas.VerifyAllComponents(ctx, cli, securesignDeployment, true)
	})

	It("Sign image with cosign cli", func(ctx SpecContext) {
		tas.VerifyByCosign(ctx, cli, securesignDeployment, prevImageName)
	})

	It("Upgrade operator", func(ctx SpecContext) {
		gomega.Eventually(func(g gomega.Gomega) error {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			catalog.UpdateSourceImage(targetedCatalogImage)
			return cli.Update(ctx, catalog.Unwrap())
		}).Should(gomega.Succeed())

		// propagate changes
		time.Sleep(time.Second)

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			return catalog.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(extension), extension.Unwrap())).To(gomega.Succeed())
			return extension.IsReady(ctx, cli) && extension.GetVersion(ctx, cli) != baseVersion
		}).WithTimeout(5 * time.Minute).Should(gomega.BeTrue())
	})

	It("Verify deployment was upgraded", func(ctx SpecContext) {
		semverBase, err := semver.Make(baseVersion)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Wait for the extension to stabilize after upgrade - OLM may briefly transition CSV states
		gomega.Eventually(func(g gomega.Gomega) {
			version := extension.GetVersion(ctx, cli)
			g.Expect(version).ToNot(gomega.BeEmpty(), "Extension version should not be empty")
			semverNew, err := semver.Make(version)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(semverNew.GT(semverBase)).To(gomega.BeTrue(), "New version should be greater than base version")
		}).Should(gomega.Succeed())

		for k, v := range map[string]string{
			fulcioAction.DeploymentName:            images.Registry.Get(images.FulcioServer),
			ctl.DeploymentName:                     images.Registry.Get(images.CTLog),
			tufAction.DeploymentName:               images.Registry.Get(images.HttpServer),
			rekorAction.ServerDeploymentName:       images.Registry.Get(images.RekorServer),
			rekorAction.SearchUiDeploymentName:     images.Registry.Get(images.RekorSearchUi),
			trillianAction.LogsignerDeploymentName: images.Registry.Get(images.TrillianLogSigner),
			trillianAction.LogserverDeploymentName: images.Registry.Get(images.TrillianServer),
			tsaAction.DeploymentName:               images.Registry.Get(images.TimestampAuthority),
		} {
			gomega.Eventually(func(g gomega.Gomega) string {
				d := &v13.Deployment{}
				g.Expect(cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      k,
				}, d)).To(gomega.Succeed())

				return d.Spec.Template.Spec.Containers[0].Image
			}).Should(gomega.Equal(v), fmt.Sprintf("Expected %s deployment image to be equal to %s", k, v))
		}

		tas.VerifyAllComponents(ctx, cli, securesignDeployment, true)
	})

	It("Verify image signature after upgrade", func(ctx SpecContext) {
		rrekor = rekor.Get(ctx, cli, namespace.Name, securesignDeployment.Name)
		gomega.Expect(rrekor).ToNot(gomega.BeNil())

		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--timestamp-certificate-chain=ts_chain.pem",
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			prevImageName,
		)).To(gomega.Succeed())
	})

	It("Sign and Verify new image after upgrade", func(ctx SpecContext) {
		tas.VerifyByCosign(ctx, cli, securesignDeployment, newImageName)
	})

	It("Make sure securesign can be deleted after upgrade", func(ctx SpecContext) {
		gomega.Eventually(func(g gomega.Gomega) {
			s := securesign.Get(ctx, cli, namespace.Name, securesignDeployment.Name)
			gomega.Expect(cli.Delete(ctx, s)).Should(gomega.Succeed())
		}).Should(gomega.Succeed())
	})
})
